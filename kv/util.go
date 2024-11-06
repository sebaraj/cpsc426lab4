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

func (h *EntryHeap) Len() int {
	return len(*h)
}

func (h *EntryHeap) Less(i, j int) bool {
	return (*h)[i].ttl < (*h)[j].ttl
}

func (h *EntryHeap) Swap(i, j int) {
	(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
	// Update the index fields
	(*h)[i].index = i
	(*h)[j].index = j
}

func (h *EntryHeap) Push(x interface{}) {
	entry := x.(*entry)
	// Append the new entry and set its index
	entry.index = len(*h)
	*h = append(*h, entry)
}

func (h *EntryHeap) Pop() interface{} {
	old := *h
	n := len(old)
	// Get the last entry
	entry := old[n-1]
	// Remove reference to avoid memory leak
	old[n-1] = nil
	// Update the heap slice
	*h = old[0 : n-1]
	// Invalidate the index
	entry.index = -1
	return entry
}
