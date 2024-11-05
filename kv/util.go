package kv

import (
	"hash/fnv"
)

/// This file can be used for any common code you want to define and separate
/// out from server.go or client.go

func GetShardForKey(key string, numShards int) int {
	hasher := fnv.New32()
	hasher.Write([]byte(key))
	return int(hasher.Sum32())%numShards + 1
}

type EntryHeap []*entry

func (h EntryHeap) Len() int           { return len(h) }
func (h EntryHeap) Less(i, j int) bool { return h[i].ttl < h[j].ttl }
func (h EntryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *EntryHeap) Push(x interface{}) {
	n := len(*h)
	entry := x.(*entry)
	entry.index = n
	*h = append(*h, entry)
}

func (h *EntryHeap) Pop() interface{} {
	old := *h
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil    // avoid memory leak
	entry.index = -1  // for safety
	*h = old[0 : n-1] // shrink slice
	return entry
}
