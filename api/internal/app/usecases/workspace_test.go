package usecases_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func newWorkspaceUsecase(
	t *testing.T,
) (
	*mocks.WorkspaceLifecycleRepo,
	*mocks.WorkingTreeGitEngine,
	*mocks.ProjectRollup,
	usecases.WorkspaceUsecase,
) {
	t.Helper()
	repo := mocks.NewWorkspaceLifecycleRepo()
	git := mocks.NewWorkingTreeGitEngine()
	roll := mocks.NewProjectRollup()
	uc := usecases.NewWorkspaceUsecase(repo, git, roll)
	return repo, git, roll, uc
}

func TestWorkspaceUsecase_List(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.ListFn = func(_ context.Context) ([]domain.Workspace, error) {
		return []domain.Workspace{{ID: "w1"}, {ID: "w2"}}, nil
	}

	list, err := uc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestWorkspaceUsecase_List_Error(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.ListFn = func(_ context.Context) ([]domain.Workspace, error) {
		return nil, errors.New("boom")
	}

	_, err := uc.List(ctx)
	assert.Error(t, err)
}

func TestWorkspaceUsecase_Get(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, Branch: "b"}, nil
	}

	got, err := uc.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "b", got.Branch)
}

func TestWorkspaceUsecase_Get_Error(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}

	_, err := uc.Get(ctx, "w1")
	assert.Error(t, err)
}

func TestWorkspaceUsecase_SetMergeStrategy(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	var gotStrategy gitdomain.MergeStrategy
	repo.SetMergeStrategyFn = func(
		_ context.Context,
		_ string,
		s gitdomain.MergeStrategy,
	) (domain.Workspace, error) {
		gotStrategy = s
		return domain.Workspace{ID: "w1", MergeStrategy: s}, nil
	}

	got, err := uc.SetMergeStrategy(ctx, "w1", gitdomain.MergeStrategyRebase)
	require.NoError(t, err)
	assert.Equal(t, gitdomain.MergeStrategyRebase, gotStrategy)
	assert.Equal(t, gitdomain.MergeStrategyRebase, got.MergeStrategy)
}

func TestWorkspaceUsecase_SetMergeStrategy_Error(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.SetMergeStrategyFn = func(
		_ context.Context,
		_ string,
		_ gitdomain.MergeStrategy,
	) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}

	_, err := uc.SetMergeStrategy(ctx, "w1", gitdomain.MergeStrategyRebase)
	assert.Error(t, err)
}

func TestWorkspaceUsecase_SyncWorkingTreeState_RecomputesAndRollsUp(t *testing.T) {
	repo, git, roll, uc := newWorkspaceUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)

	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{
			ID:           id,
			RepoID:       "r1",
			WorktreePath: "/repo/x",
			ForkPointSha: "fp",
		}, nil
	}
	var summaryPath, summaryFork string
	git.WorkingTreeSummaryFn = func(
		_ context.Context,
		repoPath string,
		forkPointSha string,
	) (int, int, bool, bool, error) {
		summaryPath = repoPath
		summaryFork = forkPointSha
		return 3, 1, true, true, nil
	}
	var captured workspace.SyncInput
	repo.SyncWorkingTreeFn = func(
		_ context.Context,
		in workspace.SyncInput,
		_ time.Time,
	) (domain.Workspace, error) {
		captured = in
		return domain.Workspace{ID: in.ID, Added: in.Added, Deleted: in.Deleted, HasConflicts: in.HasConflicts}, nil
	}

	got, err := uc.SyncWorkingTreeState(ctx, "w1", now)
	require.NoError(t, err)
	assert.Equal(t, "/repo/x", summaryPath)
	assert.Equal(t, "fp", summaryFork)
	assert.Equal(t, 3, captured.Added)
	assert.Equal(t, 1, captured.Deleted)
	assert.True(t, captured.HasConflicts)
	assert.True(t, captured.HasCommits)
	assert.Equal(t, 3, got.Added)
	assert.Equal(t, "r1", roll.TouchedRepoID)
	assert.True(t, roll.Touched)
}

func TestWorkspaceUsecase_SyncWorkingTreeState_GetError(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}

	_, err := uc.SyncWorkingTreeState(ctx, "w1", time.Now())
	assert.Error(t, err)
}

func TestWorkspaceUsecase_SyncWorkingTreeState_SummaryError(t *testing.T) {
	repo, git, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/x"}, nil
	}
	git.WorkingTreeSummaryFn = func(
		_ context.Context,
		_ string,
		_ string,
	) (int, int, bool, bool, error) {
		return 0, 0, false, false, errors.New("boom")
	}

	_, err := uc.SyncWorkingTreeState(ctx, "w1", time.Now())
	assert.Error(t, err)
}

func TestWorkspaceUsecase_SyncWorkingTreeState_SyncError(t *testing.T) {
	repo, git, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/x"}, nil
	}
	git.WorkingTreeSummaryFn = func(
		_ context.Context,
		_ string,
		_ string,
	) (int, int, bool, bool, error) {
		return 1, 1, false, false, nil
	}
	repo.SyncWorkingTreeFn = func(
		_ context.Context,
		_ workspace.SyncInput,
		_ time.Time,
	) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}

	_, err := uc.SyncWorkingTreeState(ctx, "w1", time.Now())
	assert.Error(t, err)
}
