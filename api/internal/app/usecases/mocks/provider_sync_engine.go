package mocks

import (
	"context"

	provider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

// ProviderSyncEngine is a fake of the provider.Engine surface used by
// ProviderSyncUsecase.
type ProviderSyncEngine struct {
	PollOnViewFn func(ctx context.Context, wsID, repoPath, branch string) (provider.ProviderState, error)
}

// NewProviderSyncEngine returns an empty ProviderSyncEngine.
func NewProviderSyncEngine() *ProviderSyncEngine {
	return &ProviderSyncEngine{}
}

func (e *ProviderSyncEngine) PollOnView(
	ctx context.Context,
	wsID string,
	repoPath string,
	branch string,
) (provider.ProviderState, error) {
	return e.PollOnViewFn(ctx, wsID, repoPath, branch)
}
