package diff

import "sync"

// summaryCacheEntries caps the cache. Keys are exact, so a stale entry is
// impossible by construction and no TTL is needed — the cap exists only so a
// long session moving across many workspaces, branches and commits cannot grow
// the map without bound. Oldest key wins eviction; 32 covers the handful of
// (workspace, base ref) pairs a session realistically cycles between.
const summaryCacheEntries = 32

type summaryKey struct {
	repoPath string
	ref      string
	headSHA  string
}

// summaryCache memoises the committed half of the file-summary counts:
// `git diff --numstat -M -z <ref> <headSHA> --`, which is O(diff size) and is
// therefore the single most expensive thing on the branch-review tick path.
// Both sides of that diff are immutable trees, so the value is a pure function
// of the key and only a new commit — or a different base ref, or a different
// repo — can invalidate it. Working-tree churn, which changes on every tick
// while an agent runs, cannot.
type summaryCache struct {
	mu      sync.Mutex
	entries map[summaryKey]map[string]numCount
	order   []summaryKey
}

// get returns the counts for key, calling load exactly once per distinct key
// that is not already cached. The returned map is shared with every other
// holder of the same key and must be treated as read-only. load runs outside
// the mutex: it shells out to git, and holding a process-wide lock across a
// subprocess would serialise every repo in the daemon behind the slowest one.
// Two concurrent misses on one key therefore both compute, which costs a
// duplicate diff and never a wrong answer.
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

func (c *summaryCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *summaryCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
	c.order = nil
}
