// Package serialize provides single-writer-per-aggregate serialization shared by
// the global (chat, reviewthread) aggregate repositories.
package serialize

import (
	"hash/fnv"
	"sync"
)

const shardCount = 64

// KeyedMutex serializes work per string key so only one holder per key runs at a
// time, while different keys run fully in parallel.
//
// The global chat and reviewthread repositories share one Asynx instance (and one
// event store) across every aggregate. asynx's default shard runs 8 workers, and
// the optimistic event store reads nextVersion then Appends; concurrent commands
// on the SAME aggregate id each read the same nextVersion and the losers fail with
// a version/PK conflict. A single global mutex would fix that but kill
// cross-aggregate parallelism. KeyedMutex instead hashes the id into a fixed pool
// of mutexes, so same-aggregate commands serialize while distinct aggregates do
// not. The pool is fixed-size, so it never churns a map under load. A bounded set
// of shards means two distinct ids can collide on a shard and serialize
// needlessly, which is correct (just not maximally parallel) and rare at 64
// shards.
type KeyedMutex struct {
	shards [shardCount]sync.Mutex
}

// Lock acquires the mutex guarding key, blocking until it is free.
func (k *KeyedMutex) Lock(
	key string,
) {
	k.shards[k.index(key)].Lock()
}

// Unlock releases the mutex guarding key. It must be paired with a prior Lock of
// the same key.
func (k *KeyedMutex) Unlock(
	key string,
) {
	k.shards[k.index(key)].Unlock()
}

func (k *KeyedMutex) index(
	key string,
) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return h.Sum32() % shardCount
}
