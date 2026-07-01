// Package provider is the Git provider engine (GitHub/GitLab) that resolves
// protected-branch lists and pull-request state for open workspaces.
// All provider I/O is read-only; mutations remain in domain commands.
package provider

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/engine/provider/poll"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

// PRInfo is the resolved pull-request state for a branch.
type PRInfo = providertypes.PRInfo

// ProviderState is the full snapshot returned by a poll.
type ProviderState = providertypes.ProviderState

// ProviderCapability describes what the engine can do for a repo.
type ProviderCapability = providertypes.ProviderCapability

// GitProvider is the read-only interface both GitHub and GitLab implement.
type GitProvider interface {
	// ProtectedBranches returns the list of protected branch names for the repo.
	ProtectedBranches(
		ctx context.Context,
		repoPath string,
	) ([]string, error)

	// PullRequestForBranch returns the most relevant PR for branch, or nil if none.
	// Returns nil without calling the provider if branch has no upstream remote.
	PullRequestForBranch(
		ctx context.Context,
		repoPath string,
		branch string,
	) (*PRInfo, error)

	// OwnerAvatarURL returns the avatar URL of the repo owner (org or user).
	// Returns "" when the provider CLI is unavailable or the call fails.
	OwnerAvatarURL(
		ctx context.Context,
		repoPath string,
	) (string, error)
}

// Engine wraps detect + the right provider implementation + poller.
type Engine interface {
	// Capability returns what the engine can do for a given repo.
	Capability(
		ctx context.Context,
		repoPath string,
	) (ProviderCapability, error)

	// ProtectedBranches returns protected branch names for repoPath.
	// Falls back to DefaultProtectedBranches when the provider CLI is unavailable.
	ProtectedBranches(
		ctx context.Context,
		repoPath string,
	) ([]string, error)

	// PollOnView runs an immediate poll for a workspace and returns its state.
	PollOnView(
		ctx context.Context,
		wsID string,
		repoPath string,
		branch string,
	) (ProviderState, error)

	// StartBackgroundSweep starts the 5-minute global cron sweep.
	// The onStateChange callback is the hook Wave 3 will wire to
	// SyncProviderState on the Workspace aggregate.
	StartBackgroundSweep(
		ctx context.Context,
		workspacesFn func() []poll.SweepTarget,
		onStateChange func(wsID string, state ProviderState),
	)

	// OwnerAvatarURL returns the avatar URL of the repo owner.
	// Returns "" when the provider CLI is unavailable or the lookup fails.
	OwnerAvatarURL(ctx context.Context, repoPath string) (string, error)
}

// New constructs the provider Engine, wiring detect + GitHub/GitLab + poller.
func New() Engine {
	return newEngine()
}
