package branchreview_test

import (
	"context"
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestBranchReview_GetFiles_MergesWorkingTreeState(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{
		ID:           "ws1",
		RepoID:       "repo1",
		Branch:       "feature",
		WorktreePath: "/wt",
		ForkPointSha: "fork1",
	}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}

	var gotRef string
	var gotDirty []string
	gitEng := &mockGitEngine{
		ReviewFilesFn: func(_ context.Context, _, ref string, dirty []string) ([]gitdomain.ReviewFileSummary, error) {
			gotDirty = dirty
			gotRef = ref
			return []gitdomain.ReviewFileSummary{
				{Path: "committed.go", Status: gitdomain.GitFileStatusModified, Additions: 5, Deletions: 2},
				{Path: "wip.go", Status: gitdomain.GitFileStatusAdded, Additions: 3},
			}, nil
		},
		StatusFn: func(_ context.Context, _ string) (gitdomain.GitStatus, error) {
			return gitdomain.GitStatus{Files: []gitdomain.GitFile{
				{Path: "wip.go", Status: gitdomain.GitFileStatusAdded, Staged: true},
				{Path: "scratch.txt", Status: gitdomain.GitFileStatusUntracked},
			}}, nil
		},
	}

	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)

	files, err := uc.GetFiles(ctx, "ws1")
	require.NoError(t, err)

	assert.Equal(t, "fork1", gotRef, "GetFiles must diff against the recorded fork point")
	assert.Equal(t, []string{"wip.go", "scratch.txt"}, gotDirty,
		"the status GetFiles already fetches must be threaded into ReviewFiles, not fetched twice")

	byPath := make(map[string]gitdomain.ReviewFileSummary, len(files))
	for _, f := range files {
		byPath[f.Path] = f
	}
	require.Len(t, files, 3)

	// Committed-only file: no working-tree entry → not flagged uncommitted/staged.
	assert.False(t, byPath["committed.go"].Uncommitted)
	assert.False(t, byPath["committed.go"].Staged)
	assert.Equal(t, 5, byPath["committed.go"].Additions)

	// Tracked file that is also a working-tree change → uncommitted + staged.
	assert.True(t, byPath["wip.go"].Uncommitted)
	assert.True(t, byPath["wip.go"].Staged)

	// Plain untracked file: not in the diff, folded in from status as untracked.
	require.Contains(t, byPath, "scratch.txt")
	assert.Equal(t, gitdomain.GitFileStatusUntracked, byPath["scratch.txt"].Status)
	assert.True(t, byPath["scratch.txt"].Uncommitted)
	assert.Equal(t, 0, byPath["scratch.txt"].Additions)
}

func TestBranchReview_GetFiles_StatusFailureIsNonFatal(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{ID: "ws1", RepoID: "repo1", WorktreePath: "/wt", ForkPointSha: "fork1"}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	sawDirty := false
	var gotDirty []string
	gitEng := &mockGitEngine{
		ReviewFilesFn: func(_ context.Context, _, _ string, dirty []string) ([]gitdomain.ReviewFileSummary, error) {
			sawDirty, gotDirty = true, dirty
			return []gitdomain.ReviewFileSummary{
				{Path: "committed.go", Status: gitdomain.GitFileStatusModified, Additions: 1},
			}, nil
		},
		StatusFn: func(_ context.Context, _ string) (gitdomain.GitStatus, error) {
			return gitdomain.GitStatus{}, errors.New("status boom")
		},
	}

	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)

	files, err := uc.GetFiles(ctx, "ws1")
	require.NoError(t, err, "a status failure must not fail the whole summary")
	require.Len(t, files, 1)
	assert.Equal(t, "committed.go", files[0].Path)
	assert.False(t, files[0].Uncommitted)
	require.True(t, sawDirty)
	assert.Nil(t, gotDirty,
		"an unknown dirty set must be nil, not empty: empty would claim a clean tree and serve cached counts")
}

func TestBranchReview_GetFiles_MissingWorkspace_IsNotFound(t *testing.T) {
	ctx := context.Background()

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, asynxModels.ErrNotFound
		},
	}
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.GetFiles(ctx, "nope")
	require.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestBranchReview_GetFiles_ReviewFilesError(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{ID: "ws1", RepoID: "repo1", WorktreePath: "/wt", ForkPointSha: "fork1"}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	gitEng := &mockGitEngine{
		ReviewFilesFn: func(_ context.Context, _, _ string, _ []string) ([]gitdomain.ReviewFileSummary, error) {
			return nil, errors.New("git: not a repository")
		},
	}
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, err := uc.GetFiles(ctx, "ws1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperr.ErrNotFound)
}
