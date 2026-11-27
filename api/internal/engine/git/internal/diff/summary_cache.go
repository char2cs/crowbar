package diff

import "sync"

const summaryCacheEntries = 32

type summaryKey struct {
	repoPath string
	ref      string
	headSHA  string
}

type summaryCache struct {
	mu      sync.Mutex
	entries map[summaryKey]map[string]numCount
	order   []summaryKey
}

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
