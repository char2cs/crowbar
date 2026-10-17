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
	providerFac func(res DetectResult) GitProvider // injectable for tests; nil = production default
	protected   *ttlCache[[]string]                // protected branches per remote; nil = uncached
}

// newEngine builds the engine over the exec every provider subprocess goes
// through and the clock its caches expire by.
func newEngine(execFn DetectExecFn, now func() time.Time) *providerEngine {
	return &providerEngine{
		exec: execFn,
		detectFn: cachedDetect(func(ctx context.Context, repoPath string) (DetectResult, error) {
			return detectWithExec(ctx, repoPath, execFn)
		}, now),
		protected: newTTLCache[[]string](protectedCacheCap, now),
	}
}

// protectedBranches returns prov's protected branches for the repo detected as
// res, served from the per-remote cache when one is configured: every worktree
// of the repo shares the list. Failures are never cached.
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

// ProtectedBranches returns protected branch names for repoPath.
// Falls back to DefaultProtectedBranches when the CLI is unavailable.
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

// OwnerAvatarURL returns the avatar URL of the repo owner.
// Returns "" when the provider is unavailable or the lookup fails.
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

// OpenPullRequests returns the repo's open-PR head→base graph, or nil when the
// provider is unavailable. Best-effort: never fails the caller.
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

// Capability returns what the engine can do for a given repo.
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

// PollOnView runs an immediate poll for a workspace and returns its state.
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

// StartBackgroundSweep starts the 5-minute global cron sweep.
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

// sweepPollFn returns the poll.PollFn that the sweeper calls per workspace.
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

// providerFor returns the GitProvider for the repo detected as res.
func (e *providerEngine) providerFor(
	res DetectResult,
) GitProvider {
	if e.providerFac != nil {
		return e.providerFac(res)
	}
	return e.defaultProviderFor(res)
}

// defaultProviderFor returns the production GitProvider for res.
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

// pollOnce fetches protected status and PR info for a single branch.
// PR lookup errors are treated as soft degradation: the Protected field is still
// returned and the error is logged to stderr rather than propagated.
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

// isProtected checks whether branch appears in the repo's protected list.
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

// snapToPRInfo converts a poll snapshot PR to a provider PRInfo.
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

// prInfoToSnap converts a provider PRInfo to a poll snapshot.
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
