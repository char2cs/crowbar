package provider

import (
	"context"
	"os/exec"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/provider/poll"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

type PRInfo = providertypes.PRInfo

type PRLink = providertypes.PRLink

type ProviderState = providertypes.ProviderState

type ProviderCapability = providertypes.ProviderCapability

type GitProvider interface {
	ProtectedBranches(
		ctx context.Context,
		repoPath string,
	) ([]string, error)

	PullRequestForBranch(
		ctx context.Context,
		repoPath string,
		branch string,
	) (*PRInfo, error)

	OwnerAvatarURL(
		ctx context.Context,
		repoPath string,
	) (string, error)

	OpenPullRequests(
		ctx context.Context,
		repoPath string,
	) ([]PRLink, error)
}

type Engine interface {
	Capability(
		ctx context.Context,
		repoPath string,
	) (ProviderCapability, error)

	ProtectedBranches(
		ctx context.Context,
		repoPath string,
	) ([]string, error)

	PollOnView(
		ctx context.Context,
		wsID string,
		repoPath string,
		branch string,
	) (ProviderState, error)

	StartBackgroundSweep(
		ctx context.Context,
		workspacesFn func() []poll.SweepTarget,
		onStateChange func(wsID string, state ProviderState),
	)

	OwnerAvatarURL(ctx context.Context, repoPath string) (string, error)

	OpenPullRequests(ctx context.Context, repoPath string) ([]PRLink, error)
}

func New() Engine {
	return newEngine(withWaitDelay(exec.CommandContext), time.Now)
}
