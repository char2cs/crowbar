package provider_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/provider"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

// --- test helpers ---

func newProviderSyncUsecase(
	t *testing.T,
) (
	*mocks.ProviderSyncWorkspaceRepo,
	*mocks.ProviderSyncEngine,
	provider.Usecase,
) {
	t.Helper()
	wsRepo := mocks.NewProviderSyncWorkspaceRepo()
	eng := mocks.NewProviderSyncEngine()
	uc := provider.New(wsRepo, eng)
	return wsRepo, eng, uc
}

// --- SyncFromState tests ---

func TestProviderSync_SyncFromState_NilPR(t *testing.T) {
	wsRepo, _, uc := newProviderSyncUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)

	wsRepo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, Branch: "main", WorktreePath: "/repos/x"}, nil
	}
	wsRepo.SyncProviderFn = func(
		_ context.Context,
		in workspace.ProviderInput,
		_ time.Time,
	) (domain.Workspace, error) {
		assert.False(t, in.HasPR)
		assert.True(t, in.Protected)
		assert.Empty(t, in.PRUrl)
		return domain.Workspace{ID: in.ID}, nil
	}

	state := engineprovider.ProviderState{Protected: true, PR: nil}
	err := uc.SyncFromState(ctx, "w1", state, now)
	require.NoError(t, err)
}

func TestProviderSync_SyncFromState_WithPR(t *testing.T) {
	wsRepo, _, uc := newProviderSyncUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)

	wsRepo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, Branch: "feat", WorktreePath: "/repos/x"}, nil
	}
	wsRepo.SyncProviderFn = func(
		_ context.Context,
		in workspace.ProviderInput,
		_ time.Time,
	) (domain.Workspace, error) {
		assert.True(t, in.HasPR)
		assert.Equal(t, "open", in.PRStatus)
		assert.Equal(t, "https://gh/pr/1", in.PRUrl)
		assert.Equal(t, "feat title", in.PRTitle)
		assert.Equal(t, "main", in.PRTargetBranch)
		assert.False(t, in.Protected)
		return domain.Workspace{ID: in.ID}, nil
	}

	state := engineprovider.ProviderState{
		Protected: false,
		PR: &providertypes.PRInfo{
			Status:       "open",
			URL:          "https://gh/pr/1",
			Title:        "feat title",
			TargetBranch: "main",
		},
	}
	err := uc.SyncFromState(ctx, "w1", state, now)
	require.NoError(t, err)
}

func TestProviderSync_SyncFromState_SyncProviderError_Propagates(t *testing.T) {
	wsRepo, _, uc := newProviderSyncUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)

	wsRepo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	wsRepo.SyncProviderFn = func(
		_ context.Context,
		_ workspace.ProviderInput,
		_ time.Time,
	) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("asynx error")
	}

	err := uc.SyncFromState(ctx, "w1", engineprovider.ProviderState{}, now)
	assert.Error(t, err)
}

// --- PollWorkspace tests ---

func TestProviderSync_PollWorkspace_HappyPath(t *testing.T) {
	wsRepo, eng, uc := newProviderSyncUsecase(t)
	ctx := context.Background()

	wsRepo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{
			ID:           id,
			Branch:       "feat",
			WorktreePath: "/repos/x",
		}, nil
	}
	eng.PollOnViewFn = func(
		_ context.Context,
		_ string,
		_ string,
		_ string,
	) (engineprovider.ProviderState, error) {
		return engineprovider.ProviderState{
			Protected: true,
			PR: &providertypes.PRInfo{
				Status: "open",
				URL:    "https://gh/pr/42",
				Title:  "my pr",
			},
		}, nil
	}
	wsRepo.SyncProviderFn = func(
		_ context.Context,
		in workspace.ProviderInput,
		_ time.Time,
	) (domain.Workspace, error) {
		assert.True(t, in.Protected)
		assert.True(t, in.HasPR)
		return domain.Workspace{ID: in.ID}, nil
	}

	err := uc.PollWorkspace(ctx, "w1")
	require.NoError(t, err)
}

func TestProviderSync_PollWorkspace_GetError_Propagates(t *testing.T) {
	wsRepo, _, uc := newProviderSyncUsecase(t)
	ctx := context.Background()

	wsRepo.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("not found")
	}

	err := uc.PollWorkspace(ctx, "w1")
	assert.Error(t, err)
}

func TestProviderSync_PollWorkspace_PollError_Propagates(t *testing.T) {
	wsRepo, eng, uc := newProviderSyncUsecase(t)
	ctx := context.Background()

	wsRepo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, Branch: "feat", WorktreePath: "/repos/x"}, nil
	}
	eng.PollOnViewFn = func(
		_ context.Context,
		_ string,
		_ string,
		_ string,
	) (engineprovider.ProviderState, error) {
		return engineprovider.ProviderState{}, errors.New("poll failed")
	}

	err := uc.PollWorkspace(ctx, "w1")
	assert.Error(t, err)
}
