package branchreview_test

import (
	"context"
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func outlineWorkspace() domain.Workspace {
	return domain.Workspace{
		ID:           "ws1",
		RepoID:       "repo1",
		Branch:       "feature",
		WorktreePath: "/wt",
		ForkPointSha: "fork1",
		Provisioning: domain.WorkspaceProvisioned,
	}
}

func outlineWorkspaceMock() *mockWorkspace {
	ws := outlineWorkspace()
	return &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
}

func sampleOutline() []gitdomain.FileOutline {
	return []gitdomain.FileOutline{
		{Path: "a.go", Hunks: []gitdomain.HunkShape{{OldStart: 1, OldLines: 4, NewStart: 1, NewLines: 6}}},
		{Path: "b.bin", IsBinary: true},
	}
}

func TestBranchReview_GetOutline_UsesTheSameDiffRefAsGetFiles(t *testing.T) {
	ctx := context.Background()

	var outlineRepo, outlineRef, filesRef string
	gitEng := &mockGitEngine{
		ReviewOutlineFn: func(_ context.Context, repoPath, ref string) ([]gitdomain.FileOutline, error) {
			outlineRepo, outlineRef = repoPath, ref
			return sampleOutline(), nil
		},
		ReviewFilesFn: func(_ context.Context, _, ref string, _ []string) ([]gitdomain.ReviewFileSummary, error) {
			filesRef = ref
			return nil, nil
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	files, err := uc.GetOutline(ctx, "ws1", "")
	require.NoError(t, err)
	_, err = uc.GetFiles(ctx, "ws1", "")
	require.NoError(t, err)

	assert.Equal(t, "/wt", outlineRepo)
	assert.Equal(t, "fork1", outlineRef)
	assert.Equal(t, filesRef, outlineRef, "outline and files must diff against the same ref")
	require.Len(t, files, 2)
	assert.Equal(t, "a.go", files[0].Path)
	assert.True(t, files[1].IsBinary)
}

func TestBranchReview_GetOutline_CachesACleanTreeUntilHEADMoves(t *testing.T) {
	ctx := context.Background()

	head := "head1"
	calls := 0
	gitEng := &mockGitEngine{
		ReviewOutlineFn: func(_ context.Context, _, _ string) ([]gitdomain.FileOutline, error) {
			calls++
			return sampleOutline(), nil
		},
		StatusFn: func(_ context.Context, _ string) (gitdomain.GitStatus, error) {
			return gitdomain.GitStatus{}, nil
		},
		RevParseFn: func(_ context.Context, _, _ string) (string, error) { return head, nil },
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	first, err := uc.GetOutline(ctx, "ws1", "")
	require.NoError(t, err)
	second, err := uc.GetOutline(ctx, "ws1", "")
	require.NoError(t, err)
	assert.Equal(t, 1, calls, "a clean tree at the same HEAD must not re-stream the diff")
	assert.Equal(t, first, second)

	head = "head2"
	_, err = uc.GetOutline(ctx, "ws1", "")
	require.NoError(t, err)
	assert.Equal(t, 2, calls, "a new commit must invalidate the entry")
}

func TestBranchReview_GetOutline_UntrackedOnlyTreeStillCaches(t *testing.T) {
	ctx := context.Background()

	calls := 0
	gitEng := &mockGitEngine{
		ReviewOutlineFn: func(_ context.Context, _, _ string) ([]gitdomain.FileOutline, error) {
			calls++
			return sampleOutline(), nil
		},
		StatusFn: func(_ context.Context, _ string) (gitdomain.GitStatus, error) {
			return gitdomain.GitStatus{Files: []gitdomain.GitFile{
				{Path: ".DS_Store", Status: gitdomain.GitFileStatusUntracked},
			}}, nil
		},
		RevParseFn: func(_ context.Context, _, _ string) (string, error) { return "head1", nil },
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, err := uc.GetOutline(ctx, "ws1", "")
	require.NoError(t, err)
	_, err = uc.GetOutline(ctx, "ws1", "")
	require.NoError(t, err)

	assert.Equal(t, 1, calls)
}

func TestBranchReview_GetOutline_DirtyTreeIsNeverCached(t *testing.T) {
	ctx := context.Background()

	calls := 0
	gitEng := &mockGitEngine{
		ReviewOutlineFn: func(_ context.Context, _, _ string) ([]gitdomain.FileOutline, error) {
			calls++
			return sampleOutline(), nil
		},
		StatusFn: func(_ context.Context, _ string) (gitdomain.GitStatus, error) {
			return gitdomain.GitStatus{Files: []gitdomain.GitFile{
				{Path: "a.go", Status: gitdomain.GitFileStatusModified},
			}}, nil
		},
		RevParseFn: func(_ context.Context, _, _ string) (string, error) { return "head1", nil },
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, err := uc.GetOutline(ctx, "ws1", "")
	require.NoError(t, err)
	_, err = uc.GetOutline(ctx, "ws1", "")
	require.NoError(t, err)

	assert.Equal(t, 2, calls, "a dirty tree must recompute")
}

func TestBranchReview_GetOutline_UnknownStatusIsNotCached(t *testing.T) {
	ctx := context.Background()

	calls := 0
	gitEng := &mockGitEngine{
		ReviewOutlineFn: func(_ context.Context, _, _ string) ([]gitdomain.FileOutline, error) {
			calls++
			return sampleOutline(), nil
		},
		StatusFn: func(_ context.Context, _ string) (gitdomain.GitStatus, error) {
			return gitdomain.GitStatus{}, errors.New("status boom")
		},
		RevParseFn: func(_ context.Context, _, _ string) (string, error) { return "head1", nil },
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, err := uc.GetOutline(ctx, "ws1", "")
	require.NoError(t, err, "an unreadable status must not fail the outline")
	_, err = uc.GetOutline(ctx, "ws1", "")
	require.NoError(t, err)

	assert.Equal(t, 2, calls)
}

func TestBranchReview_GetOutline_CachedSliceIsNotAliasedAcrossCallers(t *testing.T) {
	ctx := context.Background()

	gitEng := &mockGitEngine{
		ReviewOutlineFn: func(_ context.Context, _, _ string) ([]gitdomain.FileOutline, error) {
			return sampleOutline(), nil
		},
		StatusFn: func(_ context.Context, _ string) (gitdomain.GitStatus, error) {
			return gitdomain.GitStatus{}, nil
		},
		RevParseFn: func(_ context.Context, _, _ string) (string, error) { return "head1", nil },
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	first, err := uc.GetOutline(ctx, "ws1", "")
	require.NoError(t, err)
	first[0].Path = "clobbered"

	second, err := uc.GetOutline(ctx, "ws1", "")
	require.NoError(t, err)
	assert.Equal(t, "a.go", second[0].Path)
}

func TestBranchReview_GetOutline_MissingWorkspace_IsNotFound(t *testing.T) {
	ctx := context.Background()

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, asynxModels.ErrNotFound
		},
	}
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.GetOutline(ctx, "missing", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestBranchReview_GetOutline_EngineFailurePropagates(t *testing.T) {
	ctx := context.Background()

	gitEng := &mockGitEngine{
		ReviewOutlineFn: func(_ context.Context, _, _ string) ([]gitdomain.FileOutline, error) {
			return nil, errors.New("git boom")
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, err := uc.GetOutline(ctx, "ws1", "")
	require.Error(t, err)
	assert.NotErrorIs(t, err, problem.ErrNotFound, "a git failure is a 500, not a 404")
}

func TestBranchReview_GetOutline_FailedLoadIsNotCached(t *testing.T) {
	ctx := context.Background()

	calls := 0
	gitEng := &mockGitEngine{
		ReviewOutlineFn: func(_ context.Context, _, _ string) ([]gitdomain.FileOutline, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("git boom")
			}
			return sampleOutline(), nil
		},
		StatusFn: func(_ context.Context, _ string) (gitdomain.GitStatus, error) {
			return gitdomain.GitStatus{}, nil
		},
		RevParseFn: func(_ context.Context, _, _ string) (string, error) { return "head1", nil },
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, err := uc.GetOutline(ctx, "ws1", "")
	require.Error(t, err)

	files, err := uc.GetOutline(ctx, "ws1", "")
	require.NoError(t, err)
	assert.Len(t, files, 2)
	assert.Equal(t, 2, calls)
}
