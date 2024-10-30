package kv

import (
	"context"
	"sync"
	"time"

	"cs426.yale.edu/lab4/kv/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type entry struct {
	value      string
	expiryTime time.Time
}

type KvServerImpl struct {
	proto.UnimplementedKvServer
	nodeName string

	shardMap   *ShardMap
	listener   *ShardMapListener
	clientPool ClientPool
	shutdown   chan struct{}

	data        map[int]map[string]*entry
	locks       map[int]*sync.RWMutex
	cleanupTick *time.Ticker

	hostedShards map[int]bool
	shardLock    sync.RWMutex
}

func (server *KvServerImpl) handleShardMapUpdate() {
	server.shardLock.Lock()
	defer server.shardLock.Unlock()

	newShards := make(map[int]bool)
	for _, shard := range server.shardMap.ShardsForNode(server.nodeName) {
		newShards[shard] = true
	}

	// Identify shards to remove and add
	shardsToRemove := make([]int, 0)
	shardsToAdd := make([]int, 0)

	for shard := range server.hostedShards {
		if !newShards[shard] {
			shardsToRemove = append(shardsToRemove, shard)
		}
	}

	for shard := range newShards {
		if !server.hostedShards[shard] {
			shardsToAdd = append(shardsToAdd, shard)
		}
	}

	// Process removals
	for _, shard := range shardsToRemove {
		server.removeShard(shard)
	}

	// Process additions
	for _, shard := range shardsToAdd {
		server.addShard(shard)
	}
}

func (server *KvServerImpl) removeShard(shard int) {
	server.locks[shard].Lock()
	defer server.locks[shard].Unlock()

	delete(server.data, shard)
	delete(server.locks, shard)
	delete(server.hostedShards, shard)
}

func (server *KvServerImpl) addShard(shard int) {
	server.locks[shard] = &sync.RWMutex{}
	server.locks[shard].Lock()
	defer server.locks[shard].Unlock()

	server.data[shard] = make(map[string]*entry)
	server.hostedShards[shard] = true

	// Copy data from other nodes
	nodes := server.shardMap.NodesForShard(shard)
	for _, node := range nodes {
		if node != server.nodeName {
			client, err := server.clientPool.GetClient(node)
			if err != nil {
				logrus.WithError(err).Errorf("Failed to get client for node %s", node)
				continue
			}

			response, err := client.GetShardContents(context.Background(), &proto.GetShardContentsRequest{Shard: int32(shard)})
			if err != nil {
				logrus.WithError(err).Errorf("Failed to get shard contents from node %s", node)
				continue
			}

			for _, item := range response.Values {
				server.data[shard][item.Key] = &entry{
					value:      item.Value,
					expiryTime: time.Unix(0, item.TtlMsRemaining),
				}
			}

			break // We only need to copy from one node
		}
	}
}

func (server *KvServerImpl) GetShardContents(
	ctx context.Context,
	request *proto.GetShardContentsRequest,
) (*proto.GetShardContentsResponse, error) {
	shard := int(request.Shard)
	if !server.isShardHosted(shard) {
		return nil, status.Errorf(codes.NotFound, "Shard not hosted on this server")
	}

	server.locks[shard].RLock()
	defer server.locks[shard].RUnlock()

	values := make([]*proto.GetShardValue, 0, len(server.data[shard]))
	for key, entry := range server.data[shard] {
		values = append(values, &proto.GetShardValue{
			Key:            key,
			Value:          entry.value,
			TtlMsRemaining: entry.expiryTime.UnixNano(),
		})
	}

	return &proto.GetShardContentsResponse{Values: values}, nil
}

// Update the isShardHosted method to use the hostedShards map
func (server *KvServerImpl) isShardHosted(shard int) bool {
	server.shardLock.RLock()
	defer server.shardLock.RUnlock()
	return server.hostedShards[shard]
}

func (server *KvServerImpl) shardMapListenLoop() {
	listener := server.listener.UpdateChannel()
	for {
		select {
		case <-server.shutdown:
			return
		case <-listener:
			server.handleShardMapUpdate()
		}
	}
}

func MakeKvServer(nodeName string, shardMap *ShardMap, clientPool ClientPool) *KvServerImpl {
	listener := shardMap.MakeListener()
	server := &KvServerImpl{
		nodeName:     nodeName,
		shardMap:     shardMap,
		listener:     &listener,
		clientPool:   clientPool,
		shutdown:     make(chan struct{}),
		data:         make(map[int]map[string]*entry),
		locks:        make(map[int]*sync.RWMutex),
		cleanupTick:  time.NewTicker(1 * time.Second),
		hostedShards: make(map[int]bool),
	}

	// Initialize hosted shards based on the initial shard map
	for _, shard := range shardMap.ShardsForNode(nodeName) {
		server.hostedShards[shard] = true
		server.data[shard] = make(map[string]*entry)
		server.locks[shard] = &sync.RWMutex{}
	}

	go server.cleanupExpiredEntries()
	go server.shardMapListenLoop()

	return server
}

func (server *KvServerImpl) Shutdown() {
	close(server.shutdown)
	server.listener.Close()
	server.cleanupTick.Stop()
}

func (server *KvServerImpl) cleanupExpiredEntries() {
	for {
		select {
		case <-server.shutdown:
			return
		case <-server.cleanupTick.C:
			now := time.Now()
			for shard := range server.data {
				server.locks[shard].Lock()
				for key, entry := range server.data[shard] {
					if now.After(entry.expiryTime) {
						delete(server.data[shard], key)
					}
				}
				server.locks[shard].Unlock()
			}
		}
	}
}

func (server *KvServerImpl) checkShardAssignment(key string) error {
	shard := GetShardForKey(key, server.shardMap.NumShards())
	if !server.isShardHosted(shard) {
		return status.Errorf(codes.NotFound, "Shard not hosted on this server")
	}
	return nil
}

// Update Get, Set, and Delete methods to use the new isShardHosted method
func (server *KvServerImpl) Get(
	ctx context.Context,
	request *proto.GetRequest,
) (*proto.GetResponse, error) {
	logrus.WithFields(
		logrus.Fields{"node": server.nodeName, "key": request.Key},
	).Trace("node received Get() request")

	if request.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "Empty key not allowed")
	}

	shard := GetShardForKey(request.Key, server.shardMap.NumShards())
	if !server.isShardHosted(shard) {
		return nil, status.Errorf(codes.NotFound, "Shard not hosted on this server")
	}

	server.locks[shard].RLock()
	defer server.locks[shard].RUnlock()

	entry, exists := server.data[shard][request.Key]
	if !exists || time.Now().After(entry.expiryTime) {
		return &proto.GetResponse{WasFound: false}, nil
	}

	return &proto.GetResponse{
		Value:    entry.value,
		WasFound: true,
	}, nil
}

func (server *KvServerImpl) Set(
	ctx context.Context,
	request *proto.SetRequest,
) (*proto.SetResponse, error) {
	logrus.WithFields(
		logrus.Fields{"node": server.nodeName, "key": request.Key},
	).Trace("node received Set() request")

	if request.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "Empty key not allowed")
	}

	shard := GetShardForKey(request.Key, server.shardMap.NumShards())
	if !server.isShardHosted(shard) {
		return nil, status.Errorf(codes.NotFound, "Shard not hosted on this server")
	}

	server.locks[shard].Lock()
	defer server.locks[shard].Unlock()

	server.data[shard][request.Key] = &entry{
		value:      request.Value,
		expiryTime: time.Now().Add(time.Duration(request.TtlMs) * time.Millisecond),
	}

	return &proto.SetResponse{}, nil
}

func (server *KvServerImpl) Delete(
	ctx context.Context,
	request *proto.DeleteRequest,
) (*proto.DeleteResponse, error) {
	logrus.WithFields(
		logrus.Fields{"node": server.nodeName, "key": request.Key},
	).Trace("node received Delete() request")

	if request.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "Empty key not allowed")
	}

	shard := GetShardForKey(request.Key, server.shardMap.NumShards())
	if !server.isShardHosted(shard) {
		return nil, status.Errorf(codes.NotFound, "Shard not hosted on this server")
	}

	server.locks[shard].Lock()
	defer server.locks[shard].Unlock()

	delete(server.data[shard], request.Key)

	return &proto.DeleteResponse{}, nil
}
