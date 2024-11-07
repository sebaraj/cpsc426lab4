package kv

import (
	"container/heap"
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
	key   string
	value string
	ttl   uint64
	index int
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

	heaps []EntryHeap
}

func (server *KvServerImpl) handleShardMapUpdate() {
	// TODO: Part C
	server.shardLock.Lock()
	logrus.Debugf("(handleShardMapUpdate): KvServerImpl %s updating shardMap", server.nodeName)
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
			logrus.Debugln("(handleShardMapUpdate): Adding shard", shard)
		}
	}

	for i := 0; i < len(addNodes); i++ {
		shard := addNodes[i]
		StN := server.shardMap.GetState().ShardsToNodes[shard]
		rIdx := rand.Intn(len(StN))
		logrus.Debugln("(handleShardMapUpdate): rIdx: ", rIdx)
		lastErr := false
		for j := 0; j < len(StN); j++ {
			node := StN[(rIdx+j)%len(StN)]
			if node == server.nodeName {
				continue
			}
			client, err := server.clientPool.GetClient(node)
			if err != nil {
				logrus.Debugln("(handleShardMapUpdate): GetClient error: ", err)
				continue
			}
			server.shardLock.Unlock()
			newKVR, err := client.GetShardContents(context.Background(), &proto.GetShardContentsRequest{Shard: int32(shard)})
			server.shardLock.Lock()
			if err != nil {
				logrus.Debugln("(handleShardMapUpdate): GetShardContents error: ", err)
				continue
			}
			logrus.Debugln("(handleShardMapUpdate): acquiring lock for shard ", shard-1)
			server.locks[shard-1].Lock()

			server.data[shard-1] = make(map[string]*entry)
			server.heaps[shard-1] = make(EntryHeap, 0)
			heap.Init(&server.heaps[shard-1])

			newKV := newKVR.Values
			for j := 0; j < len(newKV); j++ {
				key := newKV[j].Key
				value := newKV[j].Value
				ttl := uint64(time.Now().UnixMilli()) + uint64(newKV[j].TtlMsRemaining)
				// server.data[shard-1][key] = &entry{value: value, ttl: ttl}
				newEntry := &entry{
					key:   key,
					value: value,
					ttl:   ttl,
				}
				server.data[shard-1][key] = newEntry
				heap.Push(&server.heaps[shard-1], newEntry)
				logrus.Debugln("(handleShardMapUpdate): Adding key ", key, " to shard ", shard, " with value ", value, " and ttl ", ttl)
			}
			logrus.Debugln("(handleShardMapUpdate): releasing lock for shard ", shard-1)
			server.locks[shard-1].Unlock()
			lastErr = true
			break
		}
		if !lastErr {
			server.locks[shard-1].Lock()
			server.data[shard-1] = make(map[string]*entry)
			server.heaps[shard-1] = make(EntryHeap, 0)
			heap.Init(&server.heaps[shard-1])
			server.locks[shard-1].Unlock()
		}

	}

	server.hostedShards = newShards
	logrus.Debugln("(handleShardMapUpdate): releasing shardLock")
	server.shardLock.Unlock()
	deleteNodes := make([]int, 0)
	for shard := range oldShards {
		if !newShards[shard] {
			deleteNodes = append(deleteNodes, shard)
		}
	}
	logrus.Debugln("(handleShardMapUpdate): deleting old shards...")
	for i := 0; i < len(deleteNodes); i++ {
		shard := deleteNodes[i]
		server.locks[shard-1].Lock()
		// Clear the data map for the shard
		server.data[shard-1] = make(map[string]*entry)
		// Clear and reinitialize the heap for the shard
		server.heaps[shard-1] = make(EntryHeap, 0)
		heap.Init(&server.heaps[shard-1])
		// for i := range server.data[shard-1] {
		// shard2 := GetShardForKey(i, server.shardMap.NumShards())
		// if shard2 == shard {
		// delete(server.data[shard-1], i)
		// }
		// }
		server.locks[shard-1].Unlock()
	}
	logrus.Debugln("(handleShardMapUpdate): deleted old shards; updated complete")
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
			logrus.Debugln("(Clean): Acquiring lock to clean server")
			server.shardLock.Lock()
			server.cleanupTick.Stop()
			logrus.Debugln("(Clean): Wiping server data")
			for i := range server.data {
				server.locks[i].Lock()
				server.heaps[i] = nil

				for k := range server.data[i] {
					delete(server.data[i], k)
				}
				server.data[i] = nil

				server.locks[i].Unlock()
			}
			server.data = nil
			// server.hostedShards = nil
			logrus.Debugln("(Clean): Wiped server data; releasing lock")
			server.shardLock.Unlock()

			// server.heaps = nil
			// server.locks = nil
			time.Sleep(1 * time.Second)
			server.locks = nil
			return
		case <-server.cleanupTick.C:
			server.shardLock.Lock()
			for i := 0; i < len(server.data); i++ {
				current := uint64(time.Now().UnixMilli())
				server.locks[i].Lock()
				h := &server.heaps[i]
				for h.Len() > 0 {
					entry := (*h)[0] // Peek at the top of the heap
					if entry.ttl > current {
						// The earliest ttl is in the future, stop cleaning
						break
					}

					// Remove from heap
					heap.Pop(h)
					// Remove from map
					delete(server.data[i], entry.key)
					logrus.Debugln("(Clean): Deleted expired key", entry.key, "from shard", i+1)
				}
				// for key, value := range server.data[i] {
				// 	// println(value.ttl)
				// 	// println)
				// 	current := uint64(time.Now().UnixMilli())
				// 	if value.ttl < current {
				// 		logrus.Debugln("(Clean): Deleting key ", key, " with ttl ", value.ttl, " from shard ", i, " at time ", current)
				// 		delete(server.data[i], key)
				// 	}
				// }
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
		cleanupTick: time.NewTicker(3 * time.Second),
		shardLock:   sync.RWMutex{},
		heaps:       make([]EntryHeap, shardMap.NumShards()),
	}

	// for i := 0; i < len(server.data); i++ {
	// server.data[i] = make(map[string]*entry)
	// server.heaps[i] = make(EntryHeap, 0)
	// heap.Init(&server.heaps[i])
	// }
	go server.shardMapListenLoop()
	server.handleShardMapUpdate()
	go server.Clean()
	return &server
}

func (server *KvServerImpl) Shutdown() {
	server.shutdown <- struct{}{}
	// server.shardLock.Lock()
	// server.cleanupTick.Stop()
	// logrus.Debugln("(Clean): Wiping server data")
	// for i := range server.data {
	// 	server.locks[i].Lock()
	// 	server.heaps[i] = nil
	// 	for k := range server.data[i] {
	// 		delete(server.data[i], k)
	// 	}
	// 	server.data[i] = nil
	//
	// 	server.locks[i].Unlock()
	// }
	// server.shardLock.Unlock()
	server.listener.Close()
	// // runtime.GC()
	// server.heaps = nil
	// server.locks = nil
	close(server.shutdown)
}

// NOTE: CALL WITHOUT HOLDING LOCK - input is shard, not key
func (server *KvServerImpl) isShardHosted(shard int) bool {
	server.shardLock.RLock()
	defer server.shardLock.RUnlock()
	return server.hostedShards[shard]
}

// NOTE: CALL WITHOUT HOLDING LOCK - input is key, not shard
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
	if request.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "Empty key not allowed")
	}
	// logrus.WithFields(
	// 	logrus.Fields{"node": server.nodeName, "key": request.Key},
	// ).Trace("node received Get() request")
	//
	// panic("TODO: Part A")

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
	// println("(Set) Key: ", request.Key, "; Value: ", request.Value, "; TtlMs: ", request.TtlMs)
	if request.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "Empty key not allowed")
	}
	// logrus.WithFields(
	// logrus.Fields{"node": server.nodeName, "key": request.Key},
	// ).Trace("node received Set() request")

	// panic("TODO: Part A")

	if request.TtlMs < 0 {
		return nil, status.Error(codes.InvalidArgument, "TTL must be non-negative")
	}

	shard, err := server.checkShardAssignment(request.Key)
	if err != nil {
		return nil, err
	}

	server.locks[shard-1].Lock()
	defer server.locks[shard-1].Unlock()
	entry2, exists := server.data[shard-1][request.Key]
	newTTL := uint64(time.Now().UnixMilli()) + uint64(request.TtlMs) // expiration timestamp

	if exists {
		// Remove the old entry from the heap
		heap.Remove(&server.heaps[shard-1], entry2.index)
		// Update the entry
		entry2.value = request.Value
		entry2.ttl = newTTL
		// Re-insert into the heap
		heap.Push(&server.heaps[shard-1], entry2)
	} else {
		// Create a new entry
		entry2 = &entry{
			key:   request.Key,
			value: request.Value,
			ttl:   newTTL,
		}
		server.data[shard-1][request.Key] = entry2
		heap.Push(&server.heaps[shard-1], entry2)
	}

	return &proto.SetResponse{}, nil
}

func (server *KvServerImpl) Delete(
	ctx context.Context,
	request *proto.DeleteRequest,
) (*proto.DeleteResponse, error) {
	if request.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "Empty key not allowed")
	}
	// logrus.WithFields(
	// 	logrus.Fields{"node": server.nodeName, "key": request.Key},
	// ).Trace("node received Delete() request")
	//
	// panic("TODO: Part A")

	shard, err := server.checkShardAssignment(request.Key)
	if err != nil {
		return nil, err
	}

	server.locks[shard-1].Lock()
	defer server.locks[shard-1].Unlock()

	entry, exists := server.data[shard-1][request.Key]
	if exists {
		// Remove entry from heap
		heap.Remove(&server.heaps[shard-1], entry.index)
		// Remove from map
		delete(server.data[shard-1], request.Key)
	}

	// delete(server.data[shard-1], request.Key)

	return &proto.DeleteResponse{}, nil
}

func (server *KvServerImpl) GetShardContents(
	ctx context.Context,
	request *proto.GetShardContentsRequest,
) (*proto.GetShardContentsResponse, error) {
	// panic("TODO: Part C")

	shard := int(request.Shard)
	if !server.isShardHosted(shard) {
		return nil, status.Error(codes.NotFound, "Shard not hosted on this server")
	}
	server.locks[shard-1].RLock()
	defer server.locks[shard-1].RUnlock()
	kvs := make([]*proto.GetShardValue, 0)
	for k, v := range server.data[shard-1] {
		kvs = append(kvs, &proto.GetShardValue{Key: k, Value: v.value, TtlMsRemaining: int64(v.ttl) - int64(time.Now().UnixMilli())})
	}
	return &proto.GetShardContentsResponse{Values: kvs}, nil
}
