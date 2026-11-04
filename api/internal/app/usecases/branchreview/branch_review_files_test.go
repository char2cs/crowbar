package branchreview_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func TestBranchReview_GetFiles_MergesWorkingTreeState(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{
		ID:           "ws1",
		RepoID:       "repo1",
		Branch:       "feature",
		WorktreePath: "/wt",
		ForkPointSha: "fork1",
		Provisioning: domain.WorkspaceProvisioned,
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

	files, err := uc.GetFiles(ctx, "ws1", "")
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

	ws := domain.Workspace{ID: "ws1", RepoID: "repo1", WorktreePath: "/wt", ForkPointSha: "fork1", Provisioning: domain.WorkspaceProvisioned}
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

	files, err := uc.GetFiles(ctx, "ws1", "")
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

	_, err := uc.GetFiles(ctx, "nope", "")
	require.ErrorIs(t, err, problem.ErrNotFound)
}

func TestBranchReview_GetFiles_ReviewFilesError(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{ID: "ws1", RepoID: "repo1", WorktreePath: "/wt", ForkPointSha: "fork1", Provisioning: domain.WorkspaceProvisioned}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	gitEng := &mockGitEngine{
		ReviewFilesFn: func(_ context.Context, _, _ string, _ []string) ([]gitdomain.ReviewFileSummary, error) {
			return nil, errors.New("git: not a repository")
		},
	}
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, err := uc.GetFiles(ctx, "ws1", "")
	require.Error(t, err)
	assert.NotErrorIs(t, err, problem.ErrNotFound)
}

// Ref resolution moved off Get() with the diff; GetFiles is where a missing
// repo now surfaces, since that is what resolves the base to diff against.
func TestBranchReview_GetFiles_RepoNil(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{ID: "ws1", RepoID: "gone", Branch: "feat", WorktreePath: "/wt", Provisioning: domain.WorkspaceProvisioned}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	repoStore := mocks.NewRepositoryStore() // empty — FindByKey returns nil, nil

	uc := newTestUsecase(wsMock, &mockReviewThread{}, repoStore, &mockGitEngine{})

	_, err := uc.GetFiles(ctx, "ws1", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: resolve ref")
}

// Also moved with ref resolution: a repo-store failure is only reachable from
// the path that resolves a base, which is GetFiles now, not Get.
func TestBranchReview_GetFiles_RepoStoreError(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{ID: "ws1", RepoID: "r1", Branch: "feat", WorktreePath: "/wt", Provisioning: domain.WorkspaceProvisioned}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	// Use a store that errors on FindByKey
	repoStore := &errRepoStore{err: errors.New("db: connection lost")}

	uc := newTestUsecase(wsMock, &mockReviewThread{}, repoStore, &mockGitEngine{})

	_, err := uc.GetFiles(ctx, "ws1", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: resolve ref")
}

func TestBranchReview_GetFiles_ParentGetError(t *testing.T) {
	ctx := context.Background()

	child := domain.Workspace{
		ID:           "child",
		RepoID:       "r1",
		Branch:       "feat",
		WorktreePath: "/wt",
		ParentID:     "parent",
		Provisioning: domain.WorkspaceProvisioned,
	}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "child" {
				return child, nil
			}
			return domain.Workspace{}, errors.New("parent: not found")
		},
	}

	uc := newTestUsecase(wsMock, &mockReviewThread{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.GetFiles(ctx, "child", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: resolve ref")
}

// TestBranchReview_Get_FallsBackToForkPointSha verifies that when the base
// branch cannot be resolved (here: the repo row is absent from the store, so
// resolveBase errors), the review falls back to the recorded ForkPointSha rather
// than failing. The fork point is a FALLBACK, not a fast-path: when the base
// branch DOES resolve, the live merge-base is preferred (see
// TestBranchReview_Get_PrefersLiveMergeBaseOverStaleForkPoint).
func TestBranchReview_GetFiles_FallsBackToForkPointSha(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{
		ID:           "ws1",
		RepoID:       "repo1",
		Branch:       "feature",
		WorktreePath: "/wt/feature",
		ForkPointSha: "sha123",
		Provisioning: domain.WorkspaceProvisioned,
	}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	repoStore := mocks.NewRepositoryStore()

	var gotDiffRef string
	gitEng := &mockGitEngine{
		ReviewFilesFn: func(_ context.Context, _, ref string, _ []string) ([]gitdomain.ReviewFileSummary, error) {
			gotDiffRef = ref
			return nil, nil
		},
	}

	uc := newTestUsecase(wsMock, noopThreads(), repoStore, gitEng)

	_, err := uc.GetFiles(ctx, "ws1", "")

	require.NoError(t, err)
	assert.Equal(t, "sha123", gotDiffRef, "DiffAgainstRef must fall back to ForkPointSha when the base branch is unresolvable")
}

// TestBranchReview_Get_PrefersLiveMergeBaseOverStaleForkPoint pins the fix for
// the stale-base sidebar bug: even with a ForkPointSha recorded, the review must
// diff against the LIVE merge-base of the base branch's current tip and HEAD, not
// the frozen fork point — otherwise a branch rebased onto a newer base shows the
// base branch's whole advancement as bogus additions/deletions.
func TestBranchReview_GetFiles_PrefersLiveMergeBaseOverStaleForkPoint(t *testing.T) {
	ctx := context.Background()

	child := domain.Workspace{
		ID:           "child",
		RepoID:       "repo1",
		Branch:       "feat/child",
		WorktreePath: "/wt/child",
		ParentID:     "parent",
		ForkPointSha: "stale-fork-sha",
		Provisioning: domain.WorkspaceProvisioned,
	}
	parent := domain.Workspace{ID: "parent", Branch: "develop"}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			switch id {
			case "child":
				return child, nil
			case "parent":
				return parent, nil
			}
			return domain.Workspace{}, errors.New("not found")
		},
	}

	var gotDiffRef string
	gitEng := &mockGitEngine{
		MergeBaseFn: func(_ context.Context, _, a, _ string) (string, error) {
			if strings.HasPrefix(a, "origin/") {
				return "", errors.New("unknown revision")
			}
			return "live-merge-base", nil
		},
		ReviewFilesFn: func(_ context.Context, _, ref string, _ []string) ([]gitdomain.ReviewFileSummary, error) {
			gotDiffRef = ref
			return nil, nil
		},
	}

	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, err := uc.GetFiles(ctx, "child", "")

	require.NoError(t, err)
	assert.Equal(t, "live-merge-base", gotDiffRef,
		"DiffAgainstRef must receive the live merge-base, not the recorded ForkPointSha")
}

// errRepoStore is a store.Store[domain.Repository, string] that always errors on FindByKey.
type errRepoStore struct {
	err error
}

func (s *errRepoStore) Save(_ context.Context, _ domain.Repository) error { return nil }

func (s *errRepoStore) Delete(_ context.Context, _ string) error { return nil }

func (s *errRepoStore) FindByKey(_ context.Context, _ string) (*domain.Repository, error) {
	return nil, s.err
}

func (s *errRepoStore) FindAll(_ context.Context) ([]domain.Repository, error) { return nil, nil }

// TestBranchReview_GetFiles_RootUsesDefaultBranch pins the base a workspace
// with NO parent is reviewed against: the repo's default branch.
func TestBranchReview_GetFiles_RootUsesDefaultBranch(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{
		ID:            "ws1",
		RepoID:        "repo1",
		Branch:        "feature",
		WorktreePath:  "/wt/feature",
		MergeStrategy: gitdomain.MergeStrategyMerge,
		Provisioning:  domain.WorkspaceProvisioned,
	}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	repoStore := mocks.NewRepositoryStore()
	require.NoError(t, repoStore.Save(ctx, domain.Repository{ID: "repo1", DefaultBranch: "main"}))

	var gotMergeBase, gotRef string
	gitEng := &mockGitEngine{
		MergeBaseFn: func(_ context.Context, _, a, _ string) (string, error) {
			if strings.HasPrefix(a, "origin/") {
				return "", errors.New("unknown revision")
			}
			gotMergeBase = a
			return "abc123", nil
		},
		ReviewFilesFn: func(_ context.Context, _, ref string, _ []string) ([]gitdomain.ReviewFileSummary, error) {
			gotRef = ref
			return nil, nil
		},
	}

	uc := newTestUsecase(wsMock, noopThreads(), repoStore, gitEng)

	_, err := uc.GetFiles(ctx, "ws1", "")

	require.NoError(t, err)
	assert.Equal(t, "main", gotMergeBase, "MergeBase must be called with the default branch as base")
	assert.Equal(t, "abc123", gotRef, "the review must diff against the SHA MergeBase returned")
}

// TestBranchReview_GetFiles_ChildUsesParentBranch pins the PRIMARY review case:
// a child workspace is reviewed against its PARENT's branch, not the repo
// default. This is the GitHub-like step — a whole branch versus what the parent
// has — so a base that silently fell back to the default would review the wrong
// thing while still looking entirely plausible.
func TestBranchReview_GetFiles_ChildUsesParentBranch(t *testing.T) {
	ctx := context.Background()

	child := domain.Workspace{
		ID:           "child",
		RepoID:       "repo1",
		Branch:       "feat/child",
		WorktreePath: "/wt/child",
		ParentID:     "parent",
		Provisioning: domain.WorkspaceProvisioned,
	}
	parent := domain.Workspace{ID: "parent", Branch: "develop"}

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			switch id {
			case "child":
				return child, nil
			case "parent":
				return parent, nil
			}
			return domain.Workspace{}, errors.New("not found")
		},
	}
	repoStore := mocks.NewRepositoryStore()

	var gotMergeBase, gotRef string
	gitEng := &mockGitEngine{
		// No origin remote: the origin/<base> probe fails, so resolution falls
		// back to the local parent branch ref.
		MergeBaseFn: func(_ context.Context, _, a, _ string) (string, error) {
			if strings.HasPrefix(a, "origin/") {
				return "", errors.New("unknown revision")
			}
			gotMergeBase = a
			return "fork123", nil
		},
		ReviewFilesFn: func(_ context.Context, _, ref string, _ []string) ([]gitdomain.ReviewFileSummary, error) {
			gotRef = ref
			return nil, nil
		},
	}

	uc := newTestUsecase(wsMock, noopThreads(), repoStore, gitEng)

	_, err := uc.GetFiles(ctx, "child", "")

	require.NoError(t, err)
	assert.Equal(t, "develop", gotMergeBase, "MergeBase must be called with the PARENT branch as base")
	assert.Equal(t, "fork123", gotRef, "the review must diff against the SHA MergeBase returned")
}

// TestBranchReview_GetFiles_InternalGitError_IsNotNotFound pins that a genuine
// git failure surfaces as an internal error rather than a 404.
func TestBranchReview_GetFiles_InternalGitError_IsNotNotFound(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{ID: "ws1", RepoID: "repo1", Branch: "feature", WorktreePath: "/wt", Provisioning: domain.WorkspaceProvisioned}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	repoStore := mocks.NewRepositoryStore()
	_ = repoStore.Save(ctx, domain.Repository{ID: "repo1", DefaultBranch: "main"})
	gitEng := &mockGitEngine{
		ReviewFilesFn: func(_ context.Context, _, _ string, _ []string) ([]gitdomain.ReviewFileSummary, error) {
			return nil, errors.New("git: not a repository")
		},
	}
	uc := newTestUsecase(wsMock, &mockReviewThread{}, repoStore, gitEng)

	_, err := uc.GetFiles(ctx, "ws1", "")
	require.Error(t, err)
	assert.NotErrorIs(t, err, problem.ErrNotFound)
}
