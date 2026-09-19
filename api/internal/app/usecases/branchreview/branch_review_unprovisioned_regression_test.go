package branchreview_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// TestRegression_BranchReviewOnAPlaceholder_RefusesInsteadOfRunningGitNowhere
// pins the 500 every placeholder row answered when it was opened.
//
// A placeholder workspace has no worktree on disk, and its empty WorktreePath
// was handed to git as the working directory: `git merge-base <base> HEAD` ran
// wherever the daemon itself had been started, so the request came back as
// `branch review: resolve ref: merge-base: exit 128: fatal: Not a valid object
// name main` — a 500 with raw git text, from clicking a row. Worse than the
// message, the read was being answered by an unrelated repository whenever the
// daemon's own directory happened to be one.
//
// Every review read now refuses the row before touching git, with a 409-mapped
// sentinel that names the real reason.
func TestRegression_BranchReviewOnAPlaceholder_RefusesInsteadOfRunningGitNowhere(t *testing.T) {
	ctx := context.Background()
	// The shape project import records for a protected branch another checkout
	// holds: locked, no worktree, no fork point.
	placeholder := domain.Workspace{
		ID:     "ws-placeholder",
		RepoID: "r1",
		Branch: "main",
		Status: domain.WorkspaceStatusLocked,
	}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return placeholder, nil },
	}
	// Recorded rather than t.Fatal'd: the files read runs its git inside a
	// singleflight goroutine, where a Fatal would abandon the flight every waiter
	// is parked on instead of failing the test.
	var mu sync.Mutex
	var gitPaths []string
	record := func(repoPath string) {
		mu.Lock()
		defer mu.Unlock()
		gitPaths = append(gitPaths, repoPath)
	}
	git := &mockGitEngine{
		MergeBaseFn: func(_ context.Context, repoPath, _, _ string) (string, error) {
			record(repoPath)
			return "", errors.New("fatal: Not a valid object name main")
		},
		RevParseFn: func(_ context.Context, repoPath, _ string) (string, error) {
			record(repoPath)
			return "", errors.New("fatal: Not a valid object name main")
		},
	}
	repos := mocks.NewRepositoryStore()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", DefaultBranch: "main"}))
	uc := newTestUsecase(wsMock, &mockReviewThread{}, repos, git)

	t.Run("files", func(t *testing.T) {
		_, err := uc.GetFiles(ctx, placeholder.ID, "")
		require.ErrorIs(t, err, branchreview.ErrWorkspaceUnprovisioned)
		assert.Contains(t, err.Error(), "main", "the refusal names the branch it is about")
	})
	t.Run("scope", func(t *testing.T) {
		_, err := uc.GetScope(ctx, placeholder)
		require.ErrorIs(t, err, branchreview.ErrWorkspaceUnprovisioned)
	})
	t.Run("outline", func(t *testing.T) {
		_, err := uc.GetOutline(ctx, placeholder.ID, "")
		require.ErrorIs(t, err, branchreview.ErrWorkspaceUnprovisioned)
	})
	t.Run("patch", func(t *testing.T) {
		_, _, err := uc.GetPatch(ctx, placeholder.ID, "", "f.txt", 0, io.Discard)
		require.ErrorIs(t, err, branchreview.ErrWorkspaceUnprovisioned)
	})
	t.Run("search", func(t *testing.T) {
		_, _, err := uc.SearchDiff(ctx, placeholder.ID, "", "needle", gitdomain.SearchOpts{})
		require.ErrorIs(t, err, branchreview.ErrWorkspaceUnprovisioned)
	})
	t.Run("a commit-scoped read refuses too", func(t *testing.T) {
		_, err := uc.GetOutline(ctx, placeholder.ID, "abcdef1")
		require.ErrorIs(t, err, branchreview.ErrWorkspaceUnprovisioned)
	})

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, gitPaths,
		"no review read may reach git with a working directory it does not have, got %v", gitPaths)
}
