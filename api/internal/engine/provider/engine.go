package provider

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/provider/poll"
	"github.com/char2cs/crowbar/api/internal/engine/provider/providers/github"
	"github.com/char2cs/crowbar/api/internal/engine/provider/providers/gitlab"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

type providerEngine struct {
	detectFn    func(ctx context.Context, repoPath string) (DetectResult, error)
	providerFac func(kind string) GitProvider // injectable for tests; nil = production default
}

func newEngine() *providerEngine {
	return &providerEngine{
		detectFn: Detect,
	}
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

	prov := e.providerFor(res.Kind)
	if prov == nil {
		return FallbackProtectedBranches(), nil
	}

	branches, err := prov.ProtectedBranches(ctx, repoPath)
	if err != nil {
		return FallbackProtectedBranches(), nil
	}
	return branches, nil
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

	prov := e.providerFor(res.Kind)
	if prov == nil {
		return ProviderState{}, nil
	}

	return pollOnce(ctx, prov, repoPath, branch)
}

// StartBackgroundSweep starts the 60s background sweep.
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

// providerFor returns the GitProvider for a given kind string.
func (e *providerEngine) providerFor(
	kind string,
) GitProvider {
	if e.providerFac != nil {
		return e.providerFac(kind)
	}
	return defaultProviderFor(kind)
}

// defaultProviderFor returns the production GitProvider for kind.
func defaultProviderFor(
	kind string,
) GitProvider {
	switch strings.ToLower(kind) {
	case "github":
		return github.New()
	case "gitlab":
		return gitlab.New()
	default:
		return nil
	}
}

// pollOnce fetches protected status and PR info for a single branch.
// PR lookup errors are treated as soft degradation: the Protected field is still
// returned and the error is logged to stderr rather than propagated.
func pollOnce(
	ctx context.Context,
	prov GitProvider,
	repoPath string,
	branch string,
) (ProviderState, error) {
	protected, err := isProtected(ctx, prov, repoPath, branch)
	if err != nil {
		return ProviderState{}, err
	}

	pr, prErr := prov.PullRequestForBranch(ctx, repoPath, branch)
	if prErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "provider: pr lookup for %s: %v\n", branch, prErr)
	}

	return ProviderState{
		Protected: protected,
		PR:        pr,
	}, nil
}

// isProtected checks whether branch appears in the repo's protected list.
func isProtected(
	ctx context.Context,
	prov GitProvider,
	repoPath string,
	branch string,
) (bool, error) {
	branches, err := prov.ProtectedBranches(ctx, repoPath)
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
