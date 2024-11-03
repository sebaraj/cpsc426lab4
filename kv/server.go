package kv

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"cs426.yale.edu/lab4/kv/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type entry struct {
	value string
	ttl   uint64
}

type KvServerImpl struct {
	proto.UnimplementedKvServer
	nodeName string

	shardMap   *ShardMap
	listener   *ShardMapListener
	clientPool ClientPool
	shutdown   chan struct{}

	data        []map[string]*entry
	locks       []sync.RWMutex
	cleanupTick *time.Ticker

	hostedShards map[int]bool
	shardLock    sync.RWMutex
}

func (server *KvServerImpl) handleShardMapUpdate() {
	// TODO: Part C
	server.shardLock.Lock()
	updatedShards := server.shardMap.ShardsForNode(server.nodeName)
	oldShards := server.hostedShards
	newShards := make(map[int]bool, 0)

	for i := 0; i < len(updatedShards); i++ {
		newShards[updatedShards[i]] = true
	}
	addNodes := make([]int, 0)
	for shard := range newShards {
		if !oldShards[shard] {
			addNodes = append(addNodes, shard)
		}
	}

	for i := 0; i < len(addNodes); i++ {
		shard := addNodes[i]
		StN := server.shardMap.GetState().ShardsToNodes[shard]
		rIdx := rand.Intn(len(StN))
		for j := 0; j < len(StN); j++ {
			node := StN[(rIdx+j)%len(StN)]
			if node == server.nodeName {
				continue
			}
			client, err := server.clientPool.GetClient(node)
			if err != nil {
				continue
			}
			server.shardLock.Unlock()
			newKVR, err := client.GetShardContents(context.Background(), &proto.GetShardContentsRequest{Shard: int32(shard)})
			server.shardLock.Lock()
			if err != nil {
				continue
			}
			server.locks[shard-1].Lock()
			newKV := newKVR.Values
			for j := 0; j < len(newKV); j++ {
				key := newKV[j].Key
				value := newKV[j].Value
				ttl := uint64(newKV[j].TtlMsRemaining)
				server.data[shard-1][key] = &entry{value: value, ttl: ttl}
			}
			server.locks[shard-1].Unlock()
		}

	}

	server.hostedShards = newShards
	server.shardLock.Unlock()
	deleteNodes := make([]int, 0)
	for shard := range oldShards {
		if !newShards[shard] {
			deleteNodes = append(deleteNodes, shard)
		}
	}
	for i := 0; i < len(deleteNodes); i++ {
		shard := deleteNodes[i]
		server.locks[shard-1].Lock()
		for i := range server.data[shard-1] {
			key := GetShardForKey(i, server.shardMap.NumShards())
			if key == shard {
				delete(server.data[shard-1], i)
			}
		}
		server.locks[shard-1].Unlock()
	}
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

func (server *KvServerImpl) Clean() {
	for {
		select {
		case <-server.shutdown:
			return
		case <-server.cleanupTick.C:
			server.shardLock.Lock()
			for i := 0; i < len(server.data); i++ {
				server.locks[i].Lock()
				for key, value := range server.data[i] {
					// println(value.ttl)
					// println(uint64(time.Now().UnixMilli()))
					if value.ttl < uint64(time.Now().UnixMilli()) {
						delete(server.data[i], key)
					}
				}
				server.locks[i].Unlock()
			}
			server.shardLock.Unlock()
			// time.Sleep(1 * time.Second)
		}
	}
}

func MakeKvServer(nodeName string, shardMap *ShardMap, clientPool ClientPool) *KvServerImpl {
	listener := shardMap.MakeListener()
	server := KvServerImpl{
		nodeName:    nodeName,
		shardMap:    shardMap,
		listener:    &listener,
		clientPool:  clientPool,
		shutdown:    make(chan struct{}),
		data:        make([]map[string]*entry, shardMap.NumShards()),
		locks:       make([]sync.RWMutex, shardMap.NumShards()),
		cleanupTick: time.NewTicker(1 * time.Second),
		shardLock:   sync.RWMutex{},
	}

	for i := 0; i < len(server.data); i++ {
		server.data[i] = make(map[string]*entry)
	}
	go server.shardMapListenLoop()
	server.handleShardMapUpdate()
	go server.Clean()
	return &server
}

func (server *KvServerImpl) Shutdown() {
	server.shutdown <- struct{}{}
	server.listener.Close()
	server.shardLock.Lock()
	for i := 0; i < len(server.data); i++ {
		server.locks[i].Lock()
		for key := range server.data[i] {
			delete(server.data[i], key)
		}
		server.locks[i].Unlock()
	}
	server.shardLock.Unlock()
}

// NOTE: CALL WITHOUT HOLDING LOCK - input is shard, not key
func (server *KvServerImpl) isShardHosted(shard int) bool {
	server.shardLock.RLock()
	defer server.shardLock.RUnlock()
	return server.hostedShards[shard]
}

func (server *KvServerImpl) checkShardAssignment(key string) (int, error) {
	shard := GetShardForKey(key, server.shardMap.NumShards())
	if !server.isShardHosted(shard) {
		return -1, status.Errorf(codes.NotFound, "Key is not hosting within this shard/server")
	}
	return shard, nil
}

func (server *KvServerImpl) Get(
	ctx context.Context,
	request *proto.GetRequest,
) (*proto.GetResponse, error) {
	// Trace-level logging for node receiving this request (enable by running with -log-level=trace),
	// feel free to use Trace() or Debug() logging in your code to help debug tests later without
	// cluttering logs by default. See the logging section of the spec.
	logrus.WithFields(
		logrus.Fields{"node": server.nodeName, "key": request.Key},
	).Trace("node received Get() request")

	// panic("TODO: Part A")
	if request.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "Empty key not allowed")
	}
	shard, err := server.checkShardAssignment(request.Key)
	if err != nil {
		return &proto.GetResponse{Value: "", WasFound: false}, err
	}

	server.locks[shard-1].RLock()
	defer server.locks[shard-1].RUnlock()

	entry, exists := server.data[shard-1][request.Key]
	if !exists || entry.ttl < uint64(time.Now().UnixMilli()) {
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

	// panic("TODO: Part A")

	if request.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "Empty key not allowed")
	}
	shard, err := server.checkShardAssignment(request.Key)
	if err != nil {
		return nil, err
	}

	server.locks[shard-1].Lock()
	defer server.locks[shard-1].Unlock()

	server.data[shard-1][request.Key] = &entry{
		value: request.Value,
		ttl:   uint64(time.Now().UnixMilli()) + uint64(request.TtlMs),
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

	// panic("TODO: Part A")

	if request.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "Empty key not allowed")
	}

	shard, err := server.checkShardAssignment(request.Key)
	if err != nil {
		return nil, err
	}

	server.locks[shard-1].Lock()
	defer server.locks[shard-1].Unlock()

	delete(server.data[shard-1], request.Key)

	return &proto.DeleteResponse{}, nil
}

func (server *KvServerImpl) GetShardContents(
	ctx context.Context,
	request *proto.GetShardContentsRequest,
) (*proto.GetShardContentsResponse, error) {
	// panic("TODO: Part C")

	shard := request.Shard
	server.locks[shard-1].RLock()
	defer server.locks[shard-1].RUnlock()
	kvs := make([]*proto.GetShardValue, 0)
	for k, v := range server.data[shard-1] {
		kvs = append(kvs, &proto.GetShardValue{Key: k, Value: v.value, TtlMsRemaining: int64(v.ttl)})
	}
	return &proto.GetShardContentsResponse{Values: kvs}, nil
}
