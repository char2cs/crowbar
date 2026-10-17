package provider

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

func TestCachedDetect_DoesNotCacheADetectionItsCallerCancelled(t *testing.T) {
	calls := 0
	detect := cachedDetect(func(context.Context, string) (DetectResult, error) {
		calls++
		return DetectResult{Kind: "none"}, nil
	}, time.Now)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := detect(ctx, "/repo")
	require.ErrorIs(t, err, context.Canceled)
	_, err = detect(context.Background(), "/repo")
	require.NoError(t, err)
	require.Equal(t, 2, calls, "a killed git must not pin the repo to \"none\"")
}

func TestTTLCache_StaysWithinItsCap(t *testing.T) {
	c := newTTLCache[int](2, time.Now)
	for i, key := range []string{"a", "b", "c"} {
		_, err := c.load(key, func() (int, time.Duration, error) { return i + 1, time.Hour, nil })
		require.NoError(t, err)
	}
	require.LessOrEqual(t, len(c.m), 2)
	v, ok := c.get("c")
	require.True(t, ok)
	require.Equal(t, 3, v)
}

func TestProtectedBranches_CachedPerRepoAndFailuresNotCached(t *testing.T) {
	e := &providerEngine{protected: newTTLCache[[]string](8, time.Now)}
	mp := &mockProvider{protectedBranches: []string{"main"}}

	repo := DetectResult{Kind: "github", Enabled: true, Remote: "https://github.com/o/repo"}
	for range 3 {
		ok, err := e.isProtected(context.Background(), mp, repo, "/repo", "main")
		require.NoError(t, err)
		require.True(t, ok)
	}
	require.Equal(t, 1, mp.protectedCalls)

	failing := &mockProvider{protectedErr: errors.New("rate limited")}
	other := DetectResult{Kind: "github", Enabled: true, Remote: "https://github.com/o/other"}
	_, err := e.isProtected(context.Background(), failing, other, "/other", "main")
	require.Error(t, err)
	_, err = e.isProtected(context.Background(), failing, other, "/other", "main")
	require.Error(t, err)
	require.Equal(t, 2, failing.protectedCalls)
}

// execMeter fakes every provider subprocess and counts each command line, so a
// test can assert how many execs a sequence of engine calls costs.
type execMeter struct {
	mu     sync.Mutex
	remote string
	authed bool
	calls  []string
}

func (m *execMeter) exec(ctx context.Context, name string, args ...string) *exec.Cmd {
	line := filepath.Base(name) + " " + strings.Join(args, " ")
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, line)
	out, code := "main", 0
	switch {
	case strings.HasPrefix(line, "git remote get-url"):
		out = m.remote
	case strings.HasPrefix(line, "gh auth status") && !m.authed:
		code = 1
	case strings.HasPrefix(line, "gh pr list"):
		out = "[]"
	}
	return detectFakeCmd(out, code)(ctx, name, args...)
}

func (m *execMeter) count(prefix string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, c := range m.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (m *execMeter) setRemote(remote string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remote = remote
}

// worktreePair creates a repo and one linked worktree of it, the shape of two
// workspaces of one project.
func worktreePair(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	feature := filepath.Join(root, "feature")
	mustGit(t, root, "init", "-q", "-b", "main", home)
	mustGit(t, home, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init")
	mustGit(t, home, "worktree", "add", "-q", "-b", "feature", feature)
	return home, feature
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func TestRegression_WorkspaceSwitchesCostNoProviderExecsWithinTTL(t *testing.T) {
	home, feature := worktreePair(t)
	m := &execMeter{remote: "https://github.com/o/r.git", authed: true}
	clock := &fakeClock{t: time.Unix(0, 0)}
	e := newEngine(m.exec, clock.now)
	ctx := context.Background()

	// Each switch re-subscribes: both workspaces and the repo-scoped routes ask at once.
	for range 10 {
		var wg sync.WaitGroup
		for _, p := range []string{home, feature, home, feature} {
			wg.Go(func() { _, _ = e.PollOnView(ctx, "ws", p, "feature") })
		}
		wg.Go(func() { _, _ = e.ProtectedBranches(ctx, home) })
		wg.Go(func() { _, _ = e.Capability(ctx, feature) })
		wg.Wait()
		clock.t = clock.t.Add(25 * time.Second)
	}
	require.Equal(t, 1, m.count("git remote get-url"), m.calls)
	require.Equal(t, 1, m.count("gh auth status"), m.calls)
	require.Equal(t, 1, m.count("gh api repos/o/r/branches"), m.calls)

	m.setRemote("https://github.com/o/renamed.git")
	clock.t = clock.t.Add(detectTTL)
	_, err := e.PollOnView(ctx, "ws", feature, "feature")
	require.NoError(t, err)
	require.Equal(t, 2, m.count("git remote get-url"), m.calls)
	require.Equal(t, 1, m.count("gh api repos/o/renamed/branches"), "a changed remote is picked up after the TTL")
}

func TestRegression_IdlePollsReuseOneDetectionPerRepo(t *testing.T) {
	cases := []struct {
		name       string
		remote     string
		authed     bool
		detections int
	}{
		// A remote no slug can be parsed from must not re-ask git on every poll.
		{name: "unparseable remote", remote: "/srv/github.com/o/r.git", authed: true, detections: 1},
		{name: "unauthenticated", remote: "https://github.com/o/r.git", authed: false, detections: 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home, feature := worktreePair(t)
			m := &execMeter{remote: tc.remote, authed: tc.authed}
			clock := &fakeClock{t: time.Unix(0, 0)}
			e := newEngine(m.exec, clock.now)

			for range 4 {
				_, _ = e.PollOnView(context.Background(), "home", home, "main")
				_, _ = e.PollOnView(context.Background(), "feature", feature, "feature")
				clock.t = clock.t.Add(detectMissTTL)
			}
			require.Equal(t, tc.detections, m.count("git remote get-url"), m.calls)
			require.Equal(t, tc.detections, m.count("gh auth status"), m.calls)
		})
	}
}
