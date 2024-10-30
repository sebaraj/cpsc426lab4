package kv

import (
	"context"
	"errors"
	"sync"
	"time"

	"cs426.yale.edu/lab4/kv/proto"
	// "google.golang.org/grpc"
)

type Kv struct {
	shardMap   *ShardMap
	clientPool ClientPool

	mu        sync.Mutex
	rrCounter map[int]int
}

func MakeKv(shardMap *ShardMap, clientPool ClientPool) *Kv {
	return &Kv{
		shardMap:   shardMap,
		clientPool: clientPool,
		rrCounter:  make(map[int]int),
	}
}

func (kv *Kv) Get(ctx context.Context, key string) (string, bool, error) {
	shard := GetShardForKey(key, kv.shardMap.NumShards())
	nodes := kv.shardMap.NodesForShard(shard)

	if len(nodes) == 0 {
		return "", false, errors.New("no nodes available for shard")
	}

	for i := 0; i < len(nodes); i++ {
		node := kv.getNextNode(shard, nodes)
		client, err := kv.clientPool.GetClient(node)
		if err != nil {
			// try another node
			continue
		}

		response, err := client.Get(ctx, &proto.GetRequest{Key: key})
		if err == nil {
			// return the first successful response from any node
			return response.Value, response.WasFound, nil
		}
	}

	return "", false, errors.New("all nodes failed")
}

func (kv *Kv) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	shard := GetShardForKey(key, kv.shardMap.NumShards())
	nodes := kv.shardMap.NodesForShard(shard)

	if len(nodes) == 0 {
		return errors.New("no nodes available for shard")
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(nodes))

	for _, node := range nodes {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			client, err := kv.clientPool.GetClient(nodeName)
			if err != nil {
				errChan <- err
				return
			}

			_, err = client.Set(ctx, &proto.SetRequest{Key: key, Value: value, TtlMs: ttl.Milliseconds()})
			if err != nil {
				errChan <- err
			}
		}(node)
	}

	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		// Return the first error encountered
		return <-errChan
	}

	return nil
}

func (kv *Kv) Delete(ctx context.Context, key string) error {
	shard := GetShardForKey(key, kv.shardMap.NumShards())
	nodes := kv.shardMap.NodesForShard(shard)

	if len(nodes) == 0 {
		return errors.New("no nodes available for shard")
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(nodes))

	for _, node := range nodes {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			client, err := kv.clientPool.GetClient(nodeName)
			if err != nil {
				errChan <- err
				return
			}

			_, err = client.Delete(ctx, &proto.DeleteRequest{Key: key})
			if err != nil {
				errChan <- err
			}
		}(node)
	}

	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		return <-errChan // Return the first error encountered
	}

	return nil
}

func (kv *Kv) getNextNode(shard int, nodes []string) string {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	kv.rrCounter[shard] = (kv.rrCounter[shard] + 1) % len(nodes)
	return nodes[kv.rrCounter[shard]]
}
