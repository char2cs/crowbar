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
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
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
			Provisioning: domain.WorkspaceProvisioned,
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

func TestWorkspaceUsecase_SyncWorkingTreeState_ChildDiffsAgainstParentBranch(t *testing.T) {
	repo, git, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		if id == "parent" {
			return domain.Workspace{ID: "parent", Branch: "develop"}, nil
		}
		return domain.Workspace{
			ID: id, WorktreePath: "/repo/x", ParentID: "parent", ForkPointSha: "stale-fork",
			Provisioning: domain.WorkspaceProvisioned,
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

func TestWorkspaceUsecase_SyncWorkingTreeState_RootDiffsAgainstOwnBranch(t *testing.T) {
	repo, git, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{
			ID: id, WorktreePath: "/repo/x", Branch: "develop", ForkPointSha: "stale-fork",
			Provisioning: domain.WorkspaceProvisioned,
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

func TestWorkspaceUsecase_SyncWorkingTreeState_FallsBackToForkPointWhenBaseUnresolvable(t *testing.T) {
	repo, git, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		if id == "parent" {
			return domain.Workspace{ID: "parent", Branch: "gone-branch"}, nil
		}
		return domain.Workspace{
			ID: id, WorktreePath: "/repo/x", ParentID: "parent", ForkPointSha: "fork-sha",
			Provisioning: domain.WorkspaceProvisioned,
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

func TestWorkspaceUsecase_ResolveConflicts_ClearsStatusAndRollsUp(t *testing.T) {
	repo, _, roll, uc := newWorkspaceUsecase(t)
	ctx := context.Background()
	now := time.Unix(2000, 0)

	repo.ResolveConflictsFn = func(_ context.Context, id string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id, RepoID: "r1", Status: ""}, nil
	}

	got, err := uc.ResolveConflicts(ctx, "w1", now)

	require.NoError(t, err)
	assert.Empty(t, got.Status, "a resolved conflict clears the sticky pr-conflicts status")
	assert.Equal(t, "r1", roll.TouchedRepoID)
	assert.True(t, roll.Touched)
}

func TestWorkspaceUsecase_ResolveConflicts_Error(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.ResolveConflictsFn = func(_ context.Context, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}

	_, err := uc.ResolveConflicts(ctx, "w1", time.Now())

	assert.Error(t, err)
}

func TestWorkspaceUsecase_SyncWorkingTreeState_ChildParentLookupFails_FallsBackToForkPoint(t *testing.T) {
	repo, git, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		if id == "parent" {
			return domain.Workspace{}, errors.New("parent gone")
		}
		return domain.Workspace{
			ID: id, WorktreePath: "/repo/x", ParentID: "parent", ForkPointSha: "fork-sha",
			Provisioning: domain.WorkspaceProvisioned,
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
	assert.Equal(t, "fork-sha", summaryBase, "an unresolvable parent row must fall back to the recorded fork point")
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
		return domain.Workspace{ID: id, WorktreePath: "/x", Provisioning: domain.WorkspaceProvisioned}, nil
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
		return domain.Workspace{ID: id, WorktreePath: "/x", Provisioning: domain.WorkspaceProvisioned}, nil
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

func TestMergeEligibilitiesFor_NoParent(t *testing.T) {
	_, _, _, uc := newWorkspaceUsecase(t)

	ws := domain.Workspace{ID: "w1"}
	siblings := []domain.Workspace{
		{ID: "p1", Branch: "main", Status: domain.WorkspaceStatusNew},
	}

	got := uc.MergeEligibilitiesFor(context.Background(), []domain.Workspace{ws}, siblings)[0]
	assert.False(t, got.CanMergeLocally)
	assert.Empty(t, got.ParentBranch)
}

func TestMergeEligibilitiesFor_ParentLocked(t *testing.T) {
	_, _, _, uc := newWorkspaceUsecase(t)

	ws := domain.Workspace{ID: "w1", ParentID: "p1"}
	siblings := []domain.Workspace{
		{ID: "p1", Branch: "main", Status: domain.WorkspaceStatusLocked},
	}

	got := uc.MergeEligibilitiesFor(context.Background(), []domain.Workspace{ws}, siblings)[0]
	assert.False(t, got.CanMergeLocally)
	assert.Equal(t, "main", got.ParentBranch)
}

func TestMergeEligibilitiesFor_ParentDeleted(t *testing.T) {
	_, _, _, uc := newWorkspaceUsecase(t)

	ws := domain.Workspace{ID: "w1", ParentID: "p1"}
	siblings := []domain.Workspace{
		{ID: "p1", Branch: "main", Status: domain.WorkspaceStatusDeleted},
	}

	got := uc.MergeEligibilitiesFor(context.Background(), []domain.Workspace{ws}, siblings)[0]
	assert.False(t, got.CanMergeLocally)
	assert.Equal(t, "main", got.ParentBranch)
}

func TestMergeEligibilitiesFor_ParentIdle(t *testing.T) {
	_, _, _, uc := newWorkspaceUsecase(t)

	ws := domain.Workspace{ID: "w1", ParentID: "p1"}
	siblings := []domain.Workspace{
		{ID: "p1", Branch: "feature/x", Status: domain.WorkspaceStatusNew},
	}

	got := uc.MergeEligibilitiesFor(context.Background(), []domain.Workspace{ws}, siblings)[0]
	assert.True(t, got.CanMergeLocally)
	assert.Equal(t, "feature/x", got.ParentBranch)
}

func TestMergeEligibilitiesFor_ParentMissing(t *testing.T) {
	_, _, _, uc := newWorkspaceUsecase(t)

	ws := domain.Workspace{ID: "w1", ParentID: "p1"}
	siblings := []domain.Workspace{
		{ID: "p2", Branch: "main", Status: domain.WorkspaceStatusNew},
	}

	got := uc.MergeEligibilitiesFor(context.Background(), []domain.Workspace{ws}, siblings)[0]
	assert.False(t, got.CanMergeLocally)
	assert.Empty(t, got.ParentBranch)
}

func TestMergeEligibilitiesFor_BatchesTheDryRunsAndFailsOpen(t *testing.T) {
	_, git, _, uc := newWorkspaceUsecase(t)
	var asked [][]enginegit.MergePair
	git.WouldMergeConflictsFn = func(
		_ context.Context,
		repoPath string,
		pairs []enginegit.MergePair,
	) []enginegit.MergeVerdict {
		assert.Equal(t, "/wt/parent", repoPath)
		asked = append(asked, pairs)
		return []enginegit.MergeVerdict{{Conflict: true}, {Err: errors.New("no such branch")}}
	}
	siblings := []domain.Workspace{
		{ID: "p", Branch: "feature/p", WorktreePath: "/wt/parent", Status: domain.WorkspaceStatusNew},
		{ID: "locked", Branch: "main", Status: domain.WorkspaceStatusLocked},
	}
	wss := []domain.Workspace{
		{ID: "a", Branch: "feature/a", ParentID: "p"},
		{ID: "orphan"},
		{ID: "b", Branch: "feature/b", ParentID: "p"},
		{ID: "c", Branch: "feature/c", ParentID: "locked"},
	}

	got := uc.MergeEligibilitiesFor(context.Background(), wss, siblings)

	require.Len(t, got, len(wss))
	assert.Equal(t, workspace.MergeEligibility{CanMergeLocally: true, ParentBranch: "feature/p", MergeConflicts: true}, got[0])
	assert.Equal(t, workspace.MergeEligibility{}, got[1])
	assert.Equal(t, workspace.MergeEligibility{CanMergeLocally: true, ParentBranch: "feature/p"}, got[2], "an error fails open")
	assert.Equal(t, workspace.MergeEligibility{ParentBranch: "main"}, got[3], "a locked parent is never dry-run")
	assert.Equal(t, [][]enginegit.MergePair{{
		{Ours: "feature/p", Theirs: "feature/a"},
		{Ours: "feature/p", Theirs: "feature/b"},
	}}, asked, "one git read for the whole batch")
}

func TestWorkspaceUsecase_SyncWorkingTreeState_HomeSkipsGit(t *testing.T) {
	repo, git, _, uc := newWorkspaceUsecase(t)
	ctx := context.Background()

	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{
			ID:           id,
			RepoID:       "r1",
			Kind:         domain.WorkspaceKindHome,
			WorktreePath: "/project-root",
			Provisioning: domain.WorkspaceShared,
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

func TestWorkspaceUsecase_SetLockLocksWithNoOwningChatReconciler(t *testing.T) {
	repo, _, _, uc := newWorkspaceUsecase(t)
	repo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, Status: domain.WorkspaceStatusNew}, nil
	}
	repo.SetLockFn = func(_ context.Context, id string, _ *bool, _ bool) (domain.Workspace, error) {
		return domain.Workspace{ID: id, Status: domain.WorkspaceStatusLocked}, nil
	}

	lock := true
	got, err := uc.SetLock(context.Background(), "w1", &lock)

	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatusLocked, got.Status)
}
