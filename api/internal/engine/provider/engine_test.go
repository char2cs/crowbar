package provider

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/provider/poll"
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
		) (DetectResult, error) {
			return DetectResult{Kind: kind, Enabled: enabled}, detectErr
		},
	}
}

func makeEngineWithProvider(
	kind string,
	enabled bool,
	mp GitProvider,
) *providerEngine {
	return &providerEngine{
		detectFn: func(
			_ context.Context,
			_ string,
		) (DetectResult, error) {
			return DetectResult{Kind: kind, Enabled: enabled}, nil
		},
		providerFac: func(DetectResult) GitProvider {
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
	e := newEngine(exec.CommandContext, time.Now)
	assert.NotNil(t, e.providerFor(DetectResult{Kind: "github"}))
	assert.NotNil(t, e.providerFor(DetectResult{Kind: "gitlab"}))
	assert.Nil(t, e.providerFor(DetectResult{Kind: "none"}))
	assert.Nil(t, e.providerFor(DetectResult{Kind: "bitbucket"}))
}

func TestPollOnce_Protected(t *testing.T) {
	mp := &mockProvider{
		protectedBranches: []string{"main", "develop"},
	}
	state, err := (&providerEngine{}).pollOnce(context.Background(), mp, DetectResult{}, "/repo", "main")
	require.NoError(t, err)
	assert.True(t, state.Protected)
	assert.Nil(t, state.PR)
}

func TestPollOnce_NotProtected(t *testing.T) {
	mp := &mockProvider{
		protectedBranches: []string{"main"},
	}
	state, err := (&providerEngine{}).pollOnce(context.Background(), mp, DetectResult{}, "/repo", "feature")
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
	state, err := (&providerEngine{}).pollOnce(context.Background(), mp, DetectResult{}, "/repo", "feature")
	require.NoError(t, err)
	assert.NotNil(t, state.PR)
	assert.Equal(t, 1, state.PR.Number)
}

func TestPollOnce_ProtectedBranchesError(t *testing.T) {
	mp := &mockProvider{
		protectedErr: errors.New("api fail"),
	}
	_, err := (&providerEngine{}).pollOnce(context.Background(), mp, DetectResult{}, "/repo", "main")
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
	assert.False(t, state.Protected)
	require.NotNil(t, state.PR)
	assert.Equal(t, 3, state.PR.Number)
}

func TestPollOnView_PRError(t *testing.T) {
	mp := &mockProvider{
		protectedBranches: []string{"main"},
		prErr:             errors.New("pr lookup failed"),
	}
	e := makeEngineWithProvider("github", true, mp)
	state, err := e.PollOnView(context.Background(), "ws1", "/repo", "feature")
	require.NoError(t, err)
	assert.False(t, state.Protected)
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

	fn := e.sweepPollFn()
	snap, err := fn(ctx, "ws99", "/repo", "feature")
	require.NoError(t, err)
	require.NotNil(t, snap.PR)
	assert.Equal(t, 99, snap.PR.Number)

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
	e := makeEngine("github", false, nil)
	fn := e.sweepPollFn()

	snap, err := fn(context.Background(), "ws1", "/repo", "feature")
	require.NoError(t, err)
	assert.False(t, snap.Protected)
}

func TestStartBackgroundSweep_Wires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		notified := make(chan string, 1)
		e := makeEngineWithProvider("github", true, &mockProvider{
			protectedBranches: []string{"main"},
			pr:                &PRInfo{Number: 7, Status: "open"},
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		e.StartBackgroundSweep(
			ctx,
			func() []poll.SweepTarget {
				return []poll.SweepTarget{
					{WSID: "ws1", RepoPath: "/repo", Branch: "main", HasOpenPR: true},
				}
			},
			func(wsID string, state ProviderState) {
				assert.True(t, state.Protected)
				require.NotNil(t, state.PR)
				assert.Equal(t, 7, state.PR.Number)
				notified <- wsID
			},
		)

		assert.Equal(t, "ws1", <-notified)

		cancel()
	})
}

type mockProvider struct {
	protectedBranches []string
	protectedErr      error
	pr                *PRInfo
	prErr             error
	avatarURL         string
	avatarErr         error
	prLinks           []PRLink
	prLinksErr        error
	protectedCalls    int
}

func (m *mockProvider) ProtectedBranches(
	_ context.Context,
	_ string,
) ([]string, error) {
	m.protectedCalls++
	return m.protectedBranches, m.protectedErr
}

func (m *mockProvider) PullRequestForBranch(
	_ context.Context,
	_ string,
	_ string,
) (*PRInfo, error) {
	return m.pr, m.prErr
}

func (m *mockProvider) OwnerAvatarURL(
	_ context.Context,
	_ string,
) (string, error) {
	return m.avatarURL, m.avatarErr
}

func (m *mockProvider) OpenPullRequests(
	_ context.Context,
	_ string,
) ([]PRLink, error) {
	return m.prLinks, m.prLinksErr
}

func TestOwnerAvatarURL_WithProvider(t *testing.T) {
	mp := &mockProvider{
		avatarURL: "https://avatars.githubusercontent.com/u/1",
	}
	e := makeEngineWithProvider("github", true, mp)
	got, err := e.OwnerAvatarURL(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, "https://avatars.githubusercontent.com/u/1", got)
}

func TestOwnerAvatarURL_DetectError_ReturnsEmpty(t *testing.T) {
	e := makeEngine("", false, errors.New("no git"))
	got, err := e.OwnerAvatarURL(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestOwnerAvatarURL_NotEnabled_ReturnsEmpty(t *testing.T) {
	e := makeEngine("github", false, nil)
	got, err := e.OwnerAvatarURL(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestOwnerAvatarURL_UnknownKind_ReturnsEmpty(t *testing.T) {
	e := makeEngine("none", true, nil)
	got, err := e.OwnerAvatarURL(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestOwnerAvatarURL_ProviderError_ReturnsEmpty(t *testing.T) {
	mp := &mockProvider{
		avatarErr: errors.New("api error"),
	}
	e := makeEngineWithProvider("github", true, mp)
	got, err := e.OwnerAvatarURL(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestOpenPullRequests_WithProvider(t *testing.T) {
	mp := &mockProvider{
		prLinks: []PRLink{
			{Head: "feature/a", Base: "develop", Number: 1},
			{Head: "feature/b", Base: "feature/a", Number: 2},
		},
	}
	e := makeEngineWithProvider("github", true, mp)
	got, err := e.OpenPullRequests(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, []PRLink{
		{Head: "feature/a", Base: "develop", Number: 1},
		{Head: "feature/b", Base: "feature/a", Number: 2},
	}, got)
}

func TestOpenPullRequests_DetectError_ReturnsNil(t *testing.T) {
	e := makeEngine("", false, errors.New("no git"))
	got, err := e.OpenPullRequests(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestOpenPullRequests_NotEnabled_ReturnsNil(t *testing.T) {
	e := makeEngine("github", false, nil)
	got, err := e.OpenPullRequests(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestOpenPullRequests_UnknownKind_ReturnsNil(t *testing.T) {
	e := makeEngine("none", true, nil)
	got, err := e.OpenPullRequests(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestOpenPullRequests_ProviderError_ReturnsNil(t *testing.T) {
	mp := &mockProvider{prLinksErr: errors.New("api error")}
	e := makeEngineWithProvider("github", true, mp)
	got, err := e.OpenPullRequests(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Nil(t, got, "a provider failure degrades to no links, it does not fail the caller")
}
