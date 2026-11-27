package provider

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/provider/poll"
	"github.com/char2cs/crowbar/api/internal/engine/provider/providers/github"
	"github.com/char2cs/crowbar/api/internal/engine/provider/providers/gitlab"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

type providerEngine struct {
	exec        DetectExecFn
	detectFn    func(ctx context.Context, repoPath string) (DetectResult, error)
	providerFac func(res DetectResult) GitProvider
	protected   *ttlCache[[]string]
}

func newEngine(execFn DetectExecFn, now func() time.Time) *providerEngine {
	return &providerEngine{
		exec: execFn,
		detectFn: cachedDetect(func(ctx context.Context, repoPath string) (DetectResult, error) {
			return detectWithExec(ctx, repoPath, execFn)
		}, now),
		protected: newTTLCache[[]string](protectedCacheCap, now),
	}
}

func (e *providerEngine) protectedBranches(
	ctx context.Context,
	prov GitProvider,
	res DetectResult,
	repoPath string,
) ([]string, error) {
	if e.protected == nil {
		return prov.ProtectedBranches(ctx, repoPath)
	}
	entry, err := e.protected.load(res.Remote, func() ([]string, time.Duration, error) {
		branches, err := prov.ProtectedBranches(ctx, repoPath)
		return branches, protectedTTL, err
	})
	return entry.val, err
}

var _ Engine = (*providerEngine)(nil)

func (e *providerEngine) ProtectedBranches(
	ctx context.Context,
	repoPath string,
) ([]string, error) {
	res, err := e.detectFn(ctx, repoPath)
	if err != nil || !res.Enabled {
		return FallbackProtectedBranches(), nil
	}

	prov := e.providerFor(res)
	if prov == nil {
		return FallbackProtectedBranches(), nil
	}

	branches, err := e.protectedBranches(ctx, prov, res, repoPath)
	if err != nil {
		return FallbackProtectedBranches(), nil
	}
	return branches, nil
}

func (e *providerEngine) OwnerAvatarURL(
	ctx context.Context,
	repoPath string,
) (string, error) {
	res, err := e.detectFn(ctx, repoPath)
	if err != nil || !res.Enabled {
		return "", nil
	}
	prov := e.providerFor(res)
	if prov == nil {
		return "", nil
	}
	avatarURL, err := prov.OwnerAvatarURL(ctx, repoPath)
	if err != nil {
		return "", nil
	}
	return avatarURL, nil
}

func (e *providerEngine) OpenPullRequests(
	ctx context.Context,
	repoPath string,
) ([]providertypes.PRLink, error) {
	res, err := e.detectFn(ctx, repoPath)
	if err != nil || !res.Enabled {
		return nil, nil
	}
	prov := e.providerFor(res)
	if prov == nil {
		return nil, nil
	}
	links, err := prov.OpenPullRequests(ctx, repoPath)
	if err != nil {
		return nil, nil
	}
	return links, nil
}

func (e *providerEngine) Capability(
	ctx context.Context,
	repoPath string,
) (ProviderCapability, error) {
	res, err := e.detectFn(ctx, repoPath)
	if err != nil {
		return ProviderCapability{Kind: "none"}, nil
	}
	return ProviderCapability{
		Kind:    res.Kind,
		Enabled: res.Enabled,
	}, nil
}

func (e *providerEngine) PollOnView(
	ctx context.Context,
	_ string,
	repoPath string,
	branch string,
) (ProviderState, error) {
	res, err := e.detectFn(ctx, repoPath)
	if err != nil || !res.Enabled {
		return ProviderState{}, nil
	}

	prov := e.providerFor(res)
	if prov == nil {
		return ProviderState{}, nil
	}

	return e.pollOnce(ctx, prov, res, repoPath, branch)
}

func (e *providerEngine) StartBackgroundSweep(
	ctx context.Context,
	workspacesFn func() []poll.SweepTarget,
	onStateChange func(wsID string, state ProviderState),
) {
	wrapped := func(
		wsID string,
		snap poll.ProviderStateSnapshot,
	) {
		onStateChange(wsID, ProviderState{
			Protected: snap.Protected,
			PR:        snapToPRInfo(snap.PR),
		})
	}
	sweeper := poll.NewSweeper(e.sweepPollFn(), wrapped)
	sweeper.Start(ctx, workspacesFn)
}

func (e *providerEngine) sweepPollFn() poll.PollFn {
	return func(
		ctx context.Context,
		wsID string,
		repoPath string,
		branch string,
	) (poll.ProviderStateSnapshot, error) {
		state, err := e.PollOnView(ctx, wsID, repoPath, branch)
		if err != nil {
			return poll.ProviderStateSnapshot{}, err
		}
		return poll.ProviderStateSnapshot{
			Protected: state.Protected,
			PR:        prInfoToSnap(state.PR),
		}, nil
	}
}

func (e *providerEngine) providerFor(
	res DetectResult,
) GitProvider {
	if e.providerFac != nil {
		return e.providerFac(res)
	}
	return e.defaultProviderFor(res)
}

func (e *providerEngine) defaultProviderFor(
	res DetectResult,
) GitProvider {
	switch strings.ToLower(res.Kind) {
	case "github":
		return github.New(res.Remote, github.ExecFn(e.exec))
	case "gitlab":
		return gitlab.New(gitlab.ExecFn(e.exec))
	default:
		return nil
	}
}

func (e *providerEngine) pollOnce(
	ctx context.Context,
	prov GitProvider,
	res DetectResult,
	repoPath string,
	branch string,
) (ProviderState, error) {
	protected, err := e.isProtected(ctx, prov, res, repoPath, branch)
	if err != nil {
		return ProviderState{}, err
	}

	pr, prErr := prov.PullRequestForBranch(ctx, repoPath, branch)
	if prErr != nil {
		slog.DebugContext(ctx, "provider: pr lookup failed", "component", "provider", "branch", branch, "err", prErr)
	}

	return ProviderState{
		Protected: protected,
		PR:        pr,
	}, nil
}

func (e *providerEngine) isProtected(
	ctx context.Context,
	prov GitProvider,
	res DetectResult,
	repoPath string,
	branch string,
) (bool, error) {
	branches, err := e.protectedBranches(ctx, prov, res, repoPath)
	if err != nil {
		return false, err
	}

	for _, b := range branches {
		if b == branch {
			return true, nil
		}
	}
	return false, nil
}

func snapToPRInfo(
	snap *poll.PRInfoSnapshot,
) *providertypes.PRInfo {
	if snap == nil {
		return nil
	}
	return &providertypes.PRInfo{
		Number:       snap.Number,
		Status:       snap.Status,
		URL:          snap.URL,
		Title:        snap.Title,
		TargetBranch: snap.TargetBranch,
	}
}

func prInfoToSnap(
	pr *providertypes.PRInfo,
) *poll.PRInfoSnapshot {
	if pr == nil {
		return nil
	}
	return &poll.PRInfoSnapshot{
		Number:       pr.Number,
		Status:       pr.Status,
		URL:          pr.URL,
		Title:        pr.Title,
		TargetBranch: pr.TargetBranch,
	}
}
