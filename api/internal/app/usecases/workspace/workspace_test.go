package workspace_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wsrepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func newWorkspaceUsecase(
	t *testing.T,
) (
	*mocks.WorkspaceLifecycleRepo,
	*mocks.WorkingTreeGitEngine,
	*mocks.ProjectRollup,
	workspace.Usecase,
) {
	t.Helper()
	repo := mocks.NewWorkspaceLifecycleRepo()
	git := mocks.NewWorkingTreeGitEngine()
	roll := mocks.NewProjectRollup()
	// The hierarchy-only params are nil: none of this file's tests exercise the
	// worktree hierarchy (CreateChild, Reparent, ...), only the lifecycle
	// methods repo/git/roll above back — see workspace.New's doc comment.
	uc := workspace.New(repo, git, roll, nil, nil, nil, nil, nil, nil)
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

// TestWorkspaceUsecase_ListInRepo proves ListInRepo forwards the requested
// project/repo ids to the repo's ListInRepo (the real project+repo scoping is
// proven at the repo layer in TestListInRepo_ScopesToRepo — this test proves
// only that the usecase delegates to it rather than falling back to the
// unscoped List).
func TestWorkspaceUsecase_ListInRepo(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	var gotProjectID, gotRepoID string
	repo.ListInRepoFn = func(_ context.Context, projectID, repoID string) ([]domain.Workspace, error) {
		gotProjectID = projectID
		gotRepoID = repoID
		return []domain.Workspace{{ID: "w1"}}, nil
	}

	list, err := uc.ListInRepo(ctx, "p1", "r1")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "p1", gotProjectID)
	assert.Equal(t, "r1", gotRepoID)
}

func TestWorkspaceUsecase_ListInRepo_Error(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.ListInRepoFn = func(_ context.Context, _, _ string) ([]domain.Workspace, error) {
		return nil, errors.New("boom")
	}

	_, err := uc.ListInRepo(ctx, "p1", "r1")
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
	var captured wsrepo.SyncInput
	repo.SyncWorkingTreeFn = func(
		_ context.Context,
		in wsrepo.SyncInput,
		_ time.Time,
	) (domain.Workspace, error) {
		captured = in
		ws := domain.Workspace{ID: in.ID, Added: in.Added, Deleted: in.Deleted}
		if in.HasConflicts {
			ws.Status = domain.WorkspaceStatusPRConflicts
		}
		return ws, nil
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

// TestWorkspaceUsecase_SyncWorkingTreeState_ChildDiffsAgainstParentBranch pins
// that a child's summary diffs against the PARENT BRANCH NAME (so the engine
// re-derives the live merge-base), not the frozen ForkPointSha.
func TestWorkspaceUsecase_SyncWorkingTreeState_ChildDiffsAgainstParentBranch(t *testing.T) {
	repo, git, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		if id == "parent" {
			return domain.Workspace{ID: "parent", Branch: "develop"}, nil
		}
		return domain.Workspace{
			ID: id, WorktreePath: "/repo/x", ParentID: "parent", ForkPointSha: "stale-fork",
		}, nil
	}
	var summaryBase string
	git.WorkingTreeSummaryFn = func(_ context.Context, _, base string) (int, int, bool, bool, error) {
		summaryBase = base
		return 0, 0, false, false, nil
	}
	repo.SyncWorkingTreeFn = func(_ context.Context, in wsrepo.SyncInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: in.ID}, nil
	}

	_, err := uc.SyncWorkingTreeState(ctx, "w1", time.Unix(1, 0))
	require.NoError(t, err)
	assert.Equal(t, "develop", summaryBase, "child must diff against the parent branch, not the frozen fork point")
}

// TestWorkspaceUsecase_SyncWorkingTreeState_RootDiffsAgainstOwnBranch pins that a
// protected root diffs against its OWN branch (live merge-base ≈ HEAD, i.e. only
// uncommitted work), not its recorded ForkPointSha — which goes stale as the root
// advances and otherwise inflates the sidebar count for the trunk itself.
func TestWorkspaceUsecase_SyncWorkingTreeState_RootDiffsAgainstOwnBranch(t *testing.T) {
	repo, git, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{
			ID: id, WorktreePath: "/repo/x", Branch: "develop", ForkPointSha: "stale-fork",
		}, nil
	}
	var summaryBase string
	git.WorkingTreeSummaryFn = func(_ context.Context, _, base string) (int, int, bool, bool, error) {
		summaryBase = base
		return 0, 0, false, false, nil
	}
	repo.SyncWorkingTreeFn = func(_ context.Context, in wsrepo.SyncInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: in.ID}, nil
	}

	_, err := uc.SyncWorkingTreeState(ctx, "w1", time.Unix(1, 0))
	require.NoError(t, err)
	assert.Equal(t, "develop", summaryBase, "root must diff against its own branch, not the frozen fork point")
}

// TestWorkspaceUsecase_SyncWorkingTreeState_FallsBackToForkPointWhenBaseUnresolvable
// pins the fork-point fallback: when the base branch no longer resolves in the
// worktree (renamed/deleted out of band), the summary must diff against the
// recorded ForkPointSha rather than silently reporting +0/-0.
func TestWorkspaceUsecase_SyncWorkingTreeState_FallsBackToForkPointWhenBaseUnresolvable(t *testing.T) {
	repo, git, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		if id == "parent" {
			return domain.Workspace{ID: "parent", Branch: "gone-branch"}, nil
		}
		return domain.Workspace{
			ID: id, WorktreePath: "/repo/x", ParentID: "parent", ForkPointSha: "fork-sha",
		}, nil
	}
	git.RevParseFn = func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("unknown revision")
	}
	var summaryBase string
	git.WorkingTreeSummaryFn = func(_ context.Context, _, base string) (int, int, bool, bool, error) {
		summaryBase = base
		return 0, 0, false, false, nil
	}
	repo.SyncWorkingTreeFn = func(_ context.Context, in wsrepo.SyncInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: in.ID}, nil
	}

	_, err := uc.SyncWorkingTreeState(ctx, "w1", time.Unix(1, 0))
	require.NoError(t, err)
	assert.Equal(t, "fork-sha", summaryBase, "an unresolvable base branch must fall back to the recorded fork point")
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
		_ wsrepo.SyncInput,
		_ time.Time,
	) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}

	_, err := uc.SyncWorkingTreeState(ctx, "w1", time.Now())
	assert.Error(t, err)
}

func TestMergeEligibilityFor_NoParent(t *testing.T) {
	_, _, _, uc := newWorkspaceUsecase(t)

	ws := domain.Workspace{ID: "w1"}
	siblings := []domain.Workspace{
		{ID: "p1", Branch: "main", Status: domain.WorkspaceStatusNew},
	}

	got := uc.MergeEligibilityFor(context.Background(), ws, siblings)
	assert.False(t, got.CanMergeLocally)
	assert.Empty(t, got.ParentBranch)
}

func TestMergeEligibilityFor_ParentLocked(t *testing.T) {
	_, _, _, uc := newWorkspaceUsecase(t)

	ws := domain.Workspace{ID: "w1", ParentID: "p1"}
	siblings := []domain.Workspace{
		{ID: "p1", Branch: "main", Status: domain.WorkspaceStatusLocked},
	}

	got := uc.MergeEligibilityFor(context.Background(), ws, siblings)
	assert.False(t, got.CanMergeLocally)
	assert.Equal(t, "main", got.ParentBranch)
}

func TestMergeEligibilityFor_ParentDeleted(t *testing.T) {
	_, _, _, uc := newWorkspaceUsecase(t)

	ws := domain.Workspace{ID: "w1", ParentID: "p1"}
	siblings := []domain.Workspace{
		{ID: "p1", Branch: "main", Status: domain.WorkspaceStatusDeleted},
	}

	got := uc.MergeEligibilityFor(context.Background(), ws, siblings)
	assert.False(t, got.CanMergeLocally)
	assert.Equal(t, "main", got.ParentBranch)
}

func TestMergeEligibilityFor_ParentIdle(t *testing.T) {
	_, _, _, uc := newWorkspaceUsecase(t)

	ws := domain.Workspace{ID: "w1", ParentID: "p1"}
	siblings := []domain.Workspace{
		{ID: "p1", Branch: "feature/x", Status: domain.WorkspaceStatusNew},
	}

	got := uc.MergeEligibilityFor(context.Background(), ws, siblings)
	assert.True(t, got.CanMergeLocally)
	assert.Equal(t, "feature/x", got.ParentBranch)
}

func TestMergeEligibilityFor_ParentMissing(t *testing.T) {
	_, _, _, uc := newWorkspaceUsecase(t)

	ws := domain.Workspace{ID: "w1", ParentID: "p1"}
	siblings := []domain.Workspace{
		{ID: "p2", Branch: "main", Status: domain.WorkspaceStatusNew},
	}

	got := uc.MergeEligibilityFor(context.Background(), ws, siblings)
	assert.False(t, got.CanMergeLocally)
	assert.Empty(t, got.ParentBranch)
}

// TestWorkspaceUsecase_SyncWorkingTreeState_HomeSkipsGit pins the guard behind
// the "saving a file in the project home returns 500" fix. A project's home
// workspace is rooted at the PROJECT directory, which is deliberately not a git
// repository, so `git` there exits 129 "Not a git repository". The summary is
// the first step of the post-mutation resync, so that failure surfaced as a 500
// on every home file write: the bytes landed on disk and the editor still said
// "Failed to save file". A home workspace has no branch to diff and no index to
// conflict — its summary is zero by definition and git is never invoked.
func TestWorkspaceUsecase_SyncWorkingTreeState_HomeSkipsGit(t *testing.T) {
	repo, git, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{
			ID:           id,
			RepoID:       "r1",
			Kind:         domain.WorkspaceKindHome,
			WorktreePath: "/project-root",
		}, nil
	}
	summaryCalled := false
	git.WorkingTreeSummaryFn = func(
		_ context.Context,
		_ string,
		_ string,
	) (int, int, bool, bool, error) {
		summaryCalled = true
		return 0, 0, false, false, errors.New("exit status 129: not a git repository")
	}
	var captured wsrepo.SyncInput
	repo.SyncWorkingTreeFn = func(
		_ context.Context,
		in wsrepo.SyncInput,
		_ time.Time,
	) (domain.Workspace, error) {
		captured = in
		return domain.Workspace{ID: in.ID}, nil
	}

	_, err := uc.SyncWorkingTreeState(ctx, "home-1", time.Unix(1000, 0))

	require.NoError(t, err, "a home workspace resync must not fail on git")
	assert.False(t, summaryCalled, "git is never shelled out to for a non-git home")
	assert.Equal(t, "home-1", captured.ID)
	assert.Zero(t, captured.Added)
	assert.Zero(t, captured.Deleted)
	assert.False(t, captured.HasConflicts)
	assert.False(t, captured.HasCommits)
}

// SetLock records the user's own lock decision, which outranks the provider's
// protected flag from here on. The usecase's whole job is resolving what
// "protected" currently means so the command can apply the override against it —
// and the answer has to come from the STORED row, because clearing an override
// is exactly the case where nobody is passing one in.
func TestWorkspaceUsecase_SetLockPassesTheDecisionDown(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()
	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, Status: domain.WorkspaceStatusNew}, nil
	}
	var gotLocked *bool
	var gotProtected bool
	repo.SetLockFn = func(_ context.Context, id string, locked *bool, protected bool) (domain.Workspace, error) {
		gotLocked, gotProtected = locked, protected
		return domain.Workspace{ID: id, LockOverride: locked}, nil
	}

	unlock := false
	got, err := uc.SetLock(ctx, "w1", &unlock)

	require.NoError(t, err)
	require.NotNil(t, gotLocked)
	assert.False(t, *gotLocked)
	assert.False(t, gotProtected, "an unlocked, override-free row is not protected")
	require.NotNil(t, got.LockOverride)
}

func TestWorkspaceUsecase_SetLockReadsProtectedFromTheStoredRow(t *testing.T) {
	// A row that is locked while carrying NO override is locked precisely
	// because the provider said so. That is the only way to answer "is this
	// protected?" when the override is being cleared.
	repo, _, _, uc := newWorkspaceUsecase(t)
	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, Status: domain.WorkspaceStatusLocked}, nil
	}
	var gotProtected bool
	repo.SetLockFn = func(_ context.Context, id string, locked *bool, protected bool) (domain.Workspace, error) {
		gotProtected = protected
		return domain.Workspace{ID: id}, nil
	}

	_, err := uc.SetLock(context.Background(), "w1", nil)

	require.NoError(t, err)
	assert.True(t, gotProtected)
}

func TestWorkspaceUsecase_SetLockSurfacesAFailedLoad(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	repo.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}

	_, err := uc.SetLock(context.Background(), "w1", nil)

	require.Error(t, err)
}

func TestWorkspaceUsecase_SetLockSurfacesAFailedWrite(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	repo.SetLockFn = func(_ context.Context, _ string, _ *bool, _ bool) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("occ conflict")
	}

	lock := true
	_, err := uc.SetLock(context.Background(), "w1", &lock)

	require.Error(t, err)
}
