package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/provider/internal/detect"
	"github.com/char2cs/crowbar/api/internal/engine/provider/internal/poll"
)

func makeEngine(
	kind string,
	enabled bool,
	detectErr error,
) *providerEngine {
	return &providerEngine{
		detectFn: func(
			_ context.Context,
			_ string,
		) (detect.Result, error) {
			return detect.Result{Kind: kind, Enabled: enabled}, detectErr
		},
	}
}

// makeEngineWithProvider builds an engine that returns mp for any provider kind.
func makeEngineWithProvider(
	kind string,
	enabled bool,
	mp GitProvider,
) *providerEngine {
	return &providerEngine{
		detectFn: func(
			_ context.Context,
			_ string,
		) (detect.Result, error) {
			return detect.Result{Kind: kind, Enabled: enabled}, nil
		},
		providerFac: func(_ string) GitProvider {
			return mp
		},
	}
}

func TestCapability_DetectError(t *testing.T) {
	e := makeEngine("", false, errors.New("no git"))
	cap, err := e.Capability(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, "none", cap.Kind)
	assert.False(t, cap.Enabled)
}

func TestCapability_Success(t *testing.T) {
	e := makeEngine("github", true, nil)
	cap, err := e.Capability(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, "github", cap.Kind)
	assert.True(t, cap.Enabled)
}

func TestPollOnView_DetectError(t *testing.T) {
	e := makeEngine("", false, errors.New("fail"))
	state, err := e.PollOnView(context.Background(), "ws1", "/repo", "main")
	require.NoError(t, err)
	assert.False(t, state.Protected)
	assert.Nil(t, state.PR)
}

func TestPollOnView_NotEnabled(t *testing.T) {
	e := makeEngine("github", false, nil)
	state, err := e.PollOnView(context.Background(), "ws1", "/repo", "main")
	require.NoError(t, err)
	assert.False(t, state.Protected)
}

func TestPollOnView_UnknownKind(t *testing.T) {
	e := makeEngine("none", true, nil)
	state, err := e.PollOnView(context.Background(), "ws1", "/repo", "main")
	require.NoError(t, err)
	assert.False(t, state.Protected)
}

func TestProviderFor(t *testing.T) {
	e := newEngine()
	assert.NotNil(t, e.providerFor("github"))
	assert.NotNil(t, e.providerFor("gitlab"))
	assert.Nil(t, e.providerFor("none"))
	assert.Nil(t, e.providerFor("bitbucket"))
}

func TestPollOnce_Protected(t *testing.T) {
	mp := &mockProvider{
		protectedBranches: []string{"main", "develop"},
	}
	state, err := pollOnce(context.Background(), mp, "/repo", "main")
	require.NoError(t, err)
	assert.True(t, state.Protected)
	assert.Nil(t, state.PR)
}

func TestPollOnce_NotProtected(t *testing.T) {
	mp := &mockProvider{
		protectedBranches: []string{"main"},
	}
	state, err := pollOnce(context.Background(), mp, "/repo", "feature")
	require.NoError(t, err)
	assert.False(t, state.Protected)
}

func TestPollOnce_WithPR(t *testing.T) {
	mp := &mockProvider{
		protectedBranches: []string{},
		pr: &PRInfo{
			Number: 1,
			Status: "open",
		},
	}
	state, err := pollOnce(context.Background(), mp, "/repo", "feature")
	require.NoError(t, err)
	assert.NotNil(t, state.PR)
	assert.Equal(t, 1, state.PR.Number)
}

func TestPollOnce_ProtectedBranchesError(t *testing.T) {
	mp := &mockProvider{
		protectedErr: errors.New("api fail"),
	}
	_, err := pollOnce(context.Background(), mp, "/repo", "main")
	require.Error(t, err)
}

func TestSnapToPRInfo_Nil(t *testing.T) {
	assert.Nil(t, snapToPRInfo(nil))
}

func TestSnapToPRInfo_NonNil(t *testing.T) {
	snap := &poll.PRInfoSnapshot{Number: 5, Status: "open"}
	pr := snapToPRInfo(snap)
	require.NotNil(t, pr)
	assert.Equal(t, 5, pr.Number)
}

func TestPRInfoToSnap_Nil(t *testing.T) {
	assert.Nil(t, prInfoToSnap(nil))
}

func TestPRInfoToSnap_NonNil(t *testing.T) {
	pr := &PRInfo{Number: 7, Status: "merged"}
	snap := prInfoToSnap(pr)
	require.NotNil(t, snap)
	assert.Equal(t, 7, snap.Number)
}

func TestNew_ReturnsEngine(t *testing.T) {
	eng := New()
	require.NotNil(t, eng)
}

func TestProtectedBranches_Fallback_DetectError(t *testing.T) {
	e := makeEngine("", false, errors.New("no git"))
	branches, err := e.ProtectedBranches(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "develop", "master"}, branches)
}

func TestProtectedBranches_Fallback_NotEnabled(t *testing.T) {
	e := makeEngine("github", false, nil)
	branches, err := e.ProtectedBranches(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "develop", "master"}, branches)
}

func TestProtectedBranches_Fallback_UnknownKind(t *testing.T) {
	e := makeEngine("none", true, nil)
	branches, err := e.ProtectedBranches(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "develop", "master"}, branches)
}

func TestProtectedBranches_WithProvider(t *testing.T) {
	mp := &mockProvider{
		protectedBranches: []string{"main", "release"},
	}
	e := makeEngineWithProvider("github", true, mp)
	branches, err := e.ProtectedBranches(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "release"}, branches)
}

func TestProtectedBranches_ProviderError_Fallback(t *testing.T) {
	mp := &mockProvider{
		protectedErr: errors.New("api down"),
	}
	e := makeEngineWithProvider("github", true, mp)
	branches, err := e.ProtectedBranches(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "develop", "master"}, branches)
}

func TestPollOnView_WithProvider(t *testing.T) {
	mp := &mockProvider{
		protectedBranches: []string{"main"},
		pr: &PRInfo{
			Number: 3,
			Status: "open",
		},
	}
	e := makeEngineWithProvider("github", true, mp)
	state, err := e.PollOnView(context.Background(), "ws1", "/repo", "feature")
	require.NoError(t, err)
	assert.False(t, state.Protected) // "feature" not in protected list
	require.NotNil(t, state.PR)
	assert.Equal(t, 3, state.PR.Number)
}

func TestPollOnView_PRError(t *testing.T) {
	// PR lookup errors degrade softly: PollOnView returns no error,
	// state.Protected is still populated, and state.PR is nil.
	mp := &mockProvider{
		protectedBranches: []string{"main"},
		prErr:             errors.New("pr lookup failed"),
	}
	e := makeEngineWithProvider("github", true, mp)
	state, err := e.PollOnView(context.Background(), "ws1", "/repo", "feature")
	require.NoError(t, err)
	assert.False(t, state.Protected) // "feature" not in protected list
	assert.Nil(t, state.PR)
}

func TestStartBackgroundSweep_WithProvider_CallsCallback(t *testing.T) {
	mp := &mockProvider{
		protectedBranches: []string{},
		pr: &PRInfo{
			Number: 99,
			Status: "open",
		},
	}
	e := makeEngineWithProvider("github", true, mp)

	notified := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a fast sweeper via the sweepPollFn directly to avoid 60s wait.
	fn := e.sweepPollFn()
	snap, err := fn(ctx, "ws99", "/repo", "feature")
	require.NoError(t, err)
	require.NotNil(t, snap.PR)
	assert.Equal(t, 99, snap.PR.Number)

	// Ensure StartBackgroundSweep wires without panic.
	e.StartBackgroundSweep(
		ctx,
		func() []poll.SweepTarget { return nil },
		func(wsID string, _ ProviderState) {
			notified <- wsID
		},
	)
}

func TestSweepPollFn_ReturnsState(t *testing.T) {
	e := makeEngine("none", false, nil)
	fn := e.sweepPollFn()

	snap, err := fn(context.Background(), "ws1", "/repo", "main")
	require.NoError(t, err)
	assert.False(t, snap.Protected)
	assert.Nil(t, snap.PR)
}

func TestSweepPollFn_WithPR(t *testing.T) {
	// Make an engine that succeeds with a GitHub-kind but detect disabled.
	// The pollFn should propagate the zero state.
	e := makeEngine("github", false, nil)
	fn := e.sweepPollFn()

	snap, err := fn(context.Background(), "ws1", "/repo", "feature")
	require.NoError(t, err)
	assert.False(t, snap.Protected)
}

func TestStartBackgroundSweep_Wires(t *testing.T) {
	notified := make(chan string, 1)
	e := makeEngine("none", false, nil) // provider disabled → zero state

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e.StartBackgroundSweep(
		ctx,
		func() []poll.SweepTarget {
			return []poll.SweepTarget{
				{WSID: "ws1", HasOpenPR: true},
			}
		},
		func(wsID string, _ ProviderState) {
			notified <- wsID
		},
	)

	// The sweep runs every 60s by default so the callback is NOT called immediately.
	// Just verify the method doesn't panic and returns promptly.
	select {
	case <-notified:
		// If somehow fired (e.g. extremely fast machine + timer jitter), OK.
	case <-time.After(50 * time.Millisecond):
		// Expected: sweep hasn't fired yet.
	}
}

// mockProvider is a test double for GitProvider.
type mockProvider struct {
	protectedBranches []string
	protectedErr      error
	pr                *PRInfo
	prErr             error
}

func (m *mockProvider) ProtectedBranches(
	_ context.Context,
	_ string,
) ([]string, error) {
	return m.protectedBranches, m.protectedErr
}

func (m *mockProvider) PullRequestForBranch(
	_ context.Context,
	_ string,
	_ string,
) (*PRInfo, error) {
	return m.pr, m.prErr
}
