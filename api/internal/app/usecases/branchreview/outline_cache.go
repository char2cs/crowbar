package branchreview

import (
	"slices"
	"sync"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

const outlineCacheEntries = 32

type outlineKey struct {
	repoPath string
	ref      string
	headSHA  string
}

type outlineCache struct {
	mu      sync.Mutex
	entries map[outlineKey][]gitdomain.FileOutline
	order   []outlineKey
}

func (c *outlineCache) get(
	key outlineKey,
	load func() ([]gitdomain.FileOutline, error),
) ([]gitdomain.FileOutline, error) {
	if files, ok := c.lookup(key); ok {
		return files, nil
	}
	files, err := load()
	if err != nil {
		return nil, err
	}
	c.store(key, files)
	return slices.Clone(files), nil
}

func (c *outlineCache) lookup(
	key outlineKey,
) ([]gitdomain.FileOutline, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	files, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	return slices.Clone(files), true
}

func (c *outlineCache) store(
	key outlineKey,
	files []gitdomain.FileOutline,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[outlineKey][]gitdomain.FileOutline, outlineCacheEntries)
	}
	if _, exists := c.entries[key]; !exists {
		c.order = append(c.order, key)
	}
	c.entries[key] = files
	for len(c.order) > outlineCacheEntries {
		delete(c.entries, c.order[0])
		c.order = c.order[1:]
	}
}
