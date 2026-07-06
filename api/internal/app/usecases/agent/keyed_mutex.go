package agent

import (
	"hash/fnv"
	"sync"
)

const segMutexShards = 64

// segmentMutex serializes IngestHook's read -> reduce -> persist sequence per
// crowbarSegID. The reducer (engineagent.Registry) has its own internal mutex,
// but that only serializes the in-memory session_start decision, not the
// surrounding DB read (GetActiveSegmentByCrowbarID) and persist
// (SaveSegment/SaveChat) around it. Without this, two concurrent hooks for the
// SAME crowbarSegID could each read the same "active" segment snapshot before
// either persists its outcome, and each independently create a new "active"
// segment for that crowbarSegID — violating the
// ≤1-active-segment-per-CrowbarSegmentID invariant documented on
// agentchat.Store.
//
// This is a fixed-size shard pool, the same design as
// internal/app/repositories/internal/serialize.KeyedMutex, duplicated here
// (rather than imported) because Go's internal-package visibility rules do
// not let this package reach across to repositories/internal/serialize: only
// code rooted under internal/app/repositories may import it. A bounded shard
// pool never grows a map under load; two distinct crowbarSegIDs can collide on
// a shard and serialize needlessly, which is correct (just not maximally
// parallel) and rare at 64 shards.
type segmentMutex struct {
	shards [segMutexShards]sync.Mutex
}

// Lock acquires the mutex guarding key, blocking until it is free.
func (m *segmentMutex) Lock(key string) {
	m.shards[m.index(key)].Lock()
}

// Unlock releases the mutex guarding key. It must be paired with a prior Lock
// of the same key.
func (m *segmentMutex) Unlock(key string) {
	m.shards[m.index(key)].Unlock()
}

func (m *segmentMutex) index(key string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return h.Sum32() % segMutexShards
}
