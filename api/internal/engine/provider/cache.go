package provider

import (
	"context"
	"sync"
	"time"
)

// Detection and protected-branch lists change on human timescales (a login, a
// remote edit, a branch-protection rule), yet every poll tick used to re-derive
// them: a `git remote get-url` fork, a `gh auth status` network round-trip and a
// paginated branches API call per workspace per minute. Caching them per repo
// leaves one PR lookup per tick.
const (
	detectTTL         = 5 * time.Minute
	detectMissTTL     = time.Minute // a fresh `gh auth login` is picked up quickly
	protectedTTL      = 5 * time.Minute
	protectedCacheCap = 256 // repos; bounded so a long-lived daemon cannot grow it
)

type ttlEntry[V any] struct {
	val     V
	expires time.Time
}

// ttlCache is a small per-key cache with expiry and a hard size cap. Keys are
// repo paths, so the working set is tiny; on overflow expired entries go first,
// then the whole map is reset rather than tracking recency.
type ttlCache[V any] struct {
	mu  sync.Mutex
	now func() time.Time
	cap int
	m   map[string]ttlEntry[V]
}

func newTTLCache[V any](capacity int, now func() time.Time) *ttlCache[V] {
	return &ttlCache[V]{now: now, cap: capacity, m: make(map[string]ttlEntry[V])}
}

func (c *ttlCache[V]) get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || !c.now().Before(e.expires) {
		var zero V
		return zero, false
	}
	return e.val, true
}

func (c *ttlCache[V]) put(key string, val V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if len(c.m) >= c.cap {
		c.evictLocked(now)
	}
	c.m[key] = ttlEntry[V]{val: val, expires: now.Add(ttl)}
}

// evictLocked drops expired entries and, if the map is still full, all of them.
func (c *ttlCache[V]) evictLocked(now time.Time) {
	for k, e := range c.m {
		if !now.Before(e.expires) {
			delete(c.m, k)
		}
	}
	if len(c.m) >= c.cap {
		c.m = make(map[string]ttlEntry[V])
	}
}

// cachedDetect wraps a detect function with a per-repo TTL cache. Errors are not
// cached; an unauthenticated or provider-less result is, but briefly.
func cachedDetect(
	detect func(ctx context.Context, repoPath string) (DetectResult, error),
	now func() time.Time,
) func(ctx context.Context, repoPath string) (DetectResult, error) {
	cache := newTTLCache[DetectResult](protectedCacheCap, now)
	return func(ctx context.Context, repoPath string) (DetectResult, error) {
		if res, ok := cache.get(repoPath); ok {
			return res, nil
		}
		res, err := detect(ctx, repoPath)
		if err != nil {
			return res, err
		}
		ttl := detectTTL
		if !res.Enabled {
			ttl = detectMissTTL
		}
		cache.put(repoPath, res, ttl)
		return res, nil
	}
}
