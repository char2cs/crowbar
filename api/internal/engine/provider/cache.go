package provider

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	detectTTL         = 5 * time.Minute
	detectMissTTL     = time.Minute
	protectedTTL      = 5 * time.Minute
	protectedCacheCap = 256
)

type ttlEntry[V any] struct {
	val     V
	expires time.Time
}

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

func (c *ttlCache[V]) entry(key string) (ttlEntry[V], bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || !c.now().Before(e.expires) {
		return ttlEntry[V]{}, false
	}
	return e, true
}

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
		cache.putEntry(repoPath, e)
		return e.val, nil
	}
}
