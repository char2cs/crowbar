package diff

import "sync"

// summaryCacheEntries caps the cache. Keys are exact, so no TTL is needed; the cap only
// bounds a long session, and 32 covers the (workspace, base ref) pairs one cycles through.
const summaryCacheEntries = 32

type summaryKey struct {
	repoPath string
	ref      string
	headSHA  string
}

// summaryCache memoises the committed half of the file-summary counts (`git diff
// --numstat <ref> <headSHA>`), the costliest step of a review tick. Both sides are
// immutable trees, so only a new commit, base ref or repo changes the key.
type summaryCache struct {
	mu      sync.Mutex
	entries map[summaryKey]map[string]numCount
	order   []summaryKey
}

// get returns key's counts, loading each uncached key once; the shared map is read-only.
// load runs outside the mutex so one slow repo never serialises the daemon; concurrent
// misses on one key both compute, costing a duplicate diff, never a wrong answer.
func (c *summaryCache) get(
	key summaryKey,
	load func() (map[string]numCount, error),
) (map[string]numCount, error) {
	if counts, ok := c.lookup(key); ok {
		return counts, nil
	}
	counts, err := load()
	if err != nil {
		return nil, err
	}
	c.store(key, counts)
	return counts, nil
}

func (c *summaryCache) lookup(
	key summaryKey,
) (map[string]numCount, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	counts, ok := c.entries[key]
	return counts, ok
}

func (c *summaryCache) store(
	key summaryKey,
	counts map[string]numCount,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[summaryKey]map[string]numCount, summaryCacheEntries)
	}
	if _, exists := c.entries[key]; !exists {
		c.order = append(c.order, key)
	}
	c.entries[key] = counts
	for len(c.order) > summaryCacheEntries {
		delete(c.entries, c.order[0])
		c.order = c.order[1:]
	}
}
