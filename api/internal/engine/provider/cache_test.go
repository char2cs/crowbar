package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func TestCachedDetect_ServesRepeatLookupsFromCacheUntilExpiry(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	calls := 0
	detect := cachedDetect(func(context.Context, string) (DetectResult, error) {
		calls++
		return DetectResult{Kind: "github", Enabled: true}, nil
	}, clock.now)

	for range 3 {
		res, err := detect(context.Background(), "/repo")
		require.NoError(t, err)
		require.True(t, res.Enabled)
	}
	require.Equal(t, 1, calls, "a poll tick must not fork git or call gh auth again")

	clock.t = clock.t.Add(detectTTL)
	_, _ = detect(context.Background(), "/repo")
	require.Equal(t, 2, calls, "an expired entry is re-detected")
}

func TestCachedDetect_RetriesAnUnauthenticatedRepoSooner(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	calls := 0
	detect := cachedDetect(func(context.Context, string) (DetectResult, error) {
		calls++
		return DetectResult{Kind: "github", Enabled: false}, nil
	}, clock.now)

	_, _ = detect(context.Background(), "/repo")
	clock.t = clock.t.Add(detectMissTTL)
	_, _ = detect(context.Background(), "/repo")
	require.Equal(t, 2, calls, "a fresh gh login must be noticed within detectMissTTL")
}

func TestCachedDetect_NeverCachesErrors(t *testing.T) {
	calls := 0
	detect := cachedDetect(func(context.Context, string) (DetectResult, error) {
		calls++
		return DetectResult{}, errors.New("boom")
	}, time.Now)

	_, err := detect(context.Background(), "/repo")
	require.Error(t, err)
	_, err = detect(context.Background(), "/repo")
	require.Error(t, err)
	require.Equal(t, 2, calls)
}

func TestTTLCache_StaysWithinItsCap(t *testing.T) {
	c := newTTLCache[int](2, time.Now)
	c.put("a", 1, time.Hour)
	c.put("b", 2, time.Hour)
	c.put("c", 3, time.Hour)
	require.LessOrEqual(t, len(c.m), 2)
	v, ok := c.get("c")
	require.True(t, ok)
	require.Equal(t, 3, v)
}

func TestProtectedBranches_CachedPerRepoAndFailuresNotCached(t *testing.T) {
	e := &providerEngine{protected: newTTLCache[[]string](8, time.Now)}
	mp := &mockProvider{protectedBranches: []string{"main"}}

	for range 3 {
		ok, err := e.isProtected(context.Background(), mp, "/repo", "main")
		require.NoError(t, err)
		require.True(t, ok)
	}
	require.Equal(t, 1, mp.protectedCalls)

	failing := &mockProvider{protectedErr: errors.New("rate limited")}
	_, err := e.isProtected(context.Background(), failing, "/other", "main")
	require.Error(t, err)
	_, err = e.isProtected(context.Background(), failing, "/other", "main")
	require.Error(t, err)
	require.Equal(t, 2, failing.protectedCalls)
}
