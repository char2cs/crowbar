package provider

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
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

// ttlCache is a small per-key cache with expiry and a hard size cap. Keys name
// repos (paths, git dirs, remotes), so the working set is tiny; on overflow
// expired entries go first, then the whole map is reset rather than tracking
// recency.
type ttlCache[V any] struct {
	mu     sync.Mutex
	now    func() time.Time
	cap    int
	m      map[string]ttlEntry[V]
	flight singleflight.Group
}

func newTTLCache[V any](capacity int, now func() time.Time) *ttlCache[V] {
	return &ttlCache[V]{now: now, cap: capacity, m: make(map[string]ttlEntry[V])}
}

func (c *ttlCache[V]) get(key string) (V, bool) {
	e, ok := c.entry(key)
	return e.val, ok
}

// entry returns key's unexpired entry, expiry included.
func (c *ttlCache[V]) entry(key string) (ttlEntry[V], bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || !c.now().Before(e.expires) {
		return ttlEntry[V]{}, false
	}
	return e, true
}

// load returns key's entry, running fn on a miss; concurrent misses on one key
// share a single fn call. A failed fn caches nothing.
func (c *ttlCache[V]) load(
	key string,
	fn func() (V, time.Duration, error),
) (ttlEntry[V], error) {
	if e, ok := c.entry(key); ok {
		return e, nil
	}
	v, err, _ := c.flight.Do(key, func() (any, error) {
		if e, ok := c.entry(key); ok {
			return e, nil
		}
		val, ttl, err := fn()
		if err != nil {
			return nil, err
		}
		e := ttlEntry[V]{val: val, expires: c.now().Add(ttl)}
		c.putEntry(key, e)
		return e, nil
	})
	if err != nil {
		return ttlEntry[V]{}, err
	}
	e, _ := v.(ttlEntry[V])
	return e, nil
}

func (c *ttlCache[V]) putEntry(key string, e ttlEntry[V]) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= c.cap {
		c.evictLocked(c.now())
	}
	c.m[key] = e
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

// cachedDetect wraps a detect function with a TTL cache keyed by repository:
// every worktree of a repo shares one origin, so one answer, and concurrent
// misses share one detection. Errors are not cached; an unauthenticated or
// provider-less result is, but briefly.
func cachedDetect(
	detect func(ctx context.Context, repoPath string) (DetectResult, error),
	now func() time.Time,
) func(ctx context.Context, repoPath string) (DetectResult, error) {
	cache := newTTLCache[DetectResult](protectedCacheCap, now)
	return func(ctx context.Context, repoPath string) (DetectResult, error) {
		if res, ok := cache.get(repoPath); ok {
			return res, nil
		}
		e, err := cache.load(repoKey(repoPath), func() (DetectResult, time.Duration, error) {
			res, err := detect(ctx, repoPath)
			if err == nil {
				// A detection cut short by its caller degrades to "none", which is not the repo's answer.
				err = ctx.Err()
			}
			if !res.Enabled {
				return res, detectMissTTL, err
			}
			return res, detectTTL, err
		})
		if err != nil {
			return DetectResult{}, err
		}
		// The asked path shares the repo entry's expiry, so a hit costs no path resolution.
		cache.putEntry(repoPath, e)
		return e.val, nil
	}
}
