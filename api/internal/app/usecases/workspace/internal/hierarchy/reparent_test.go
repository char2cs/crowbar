package hierarchy_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// TestReparent_ConflictAbortsAndSettlesAtMergeBase proves replayAndReparent's
// conflict path: a conflicting RebaseOnto is ABORTED (never left mid-rebase),
// yet the move still lands — the child is persisted under the new parent with
// its fork point set to merge-base(newParentTip, childBranch), not the new
// parent's tip, so diffs read against the true shared history until a later
// clean rebase finalizes it.
func TestReparent_ConflictAbortsAndSettlesAtMergeBase(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	var rID, rParent, rSha string
	ws.ReparentFn = func(_ context.Context, id, parentID, forkPointSha string, _ time.Time) (domain.Workspace, error) {
		rID, rParent, rSha = id, parentID, forkPointSha
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: enginegit.ErrConflict, mergeBaseSha: "mergebase-sha"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
	assert.Equal(t, []string{"RevParse", "MergeBase", "RebaseOnto", "OperationAbort", "MergeBase", "WorkingTreeSummary"}, g.ops())
	assert.Equal(t, []string{"/cw"}, g.calls[3].args, "the abort must run in the CHILD worktree")
	assert.Equal(t, []string{"/cw", "ntip", "feat"}, g.calls[4].args, "merge-base of the new parent tip and the branch")
	assert.Equal(t, "c", rID)
	assert.Equal(t, "np", rParent)
	assert.Equal(t, "mergebase-sha", rSha, "the move lands at the MERGE-BASE, not the new parent's raw tip")
}

// TestReparent_ConflictAbortFails proves that when the post-conflict abort
// itself fails, Reparent surfaces the error and never persists the move — a
// stuck mid-rebase worktree must not be silently reported as reparented.
func TestReparent_ConflictAbortFails(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	persisted := false
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		persisted = true
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: enginegit.ErrConflict, operationAbortErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
	assert.False(t, persisted, "a failed abort must never persist a partial move")
	assert.Equal(t, []string{"RevParse", "MergeBase", "RebaseOnto", "OperationAbort"}, g.ops(),
		"no merge-base attempt after a failed abort")
}

// TestReparent_ConflictMergeBaseFails proves a merge-base resolution failure
// surfaces cleanly and never persists the move.
func TestReparent_ConflictMergeBaseFails(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	persisted := false
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		persisted = true
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: enginegit.ErrConflict, mergeBaseErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
	assert.False(t, persisted, "no move is persisted when the merge-base cannot be resolved")
}

// TestReparent_PersistError proves a clean rebase whose PERSIST step fails
// (the aggregate write) surfaces the error, even though the git-side rebase
// already succeeded.
func TestReparent_PersistError(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, _, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

// TestReparent_ResyncSummaryError_IsBestEffort proves that when the persist
// succeeds but the follow-up working-tree-summary SYNC write fails (distinct
// from a WorkingTreeSummary READ failure, which a separate test already
// covers), Reparent still reports success — the resync is best-effort and the
// read model self-corrects on the next watcher event.
func TestReparent_ResyncSummaryError_IsBestEffort(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	ws.SyncFn = func(_ context.Context, _ workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	out, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err, "a resync sync-write failure must not fail the reparent")
	assert.Equal(t, "c", out.ID)
}

// TestRegression_ConflictingReparentMarksTheChildConflicting: the aborted
// rebase leaves git clean, so only the flag tells the user the move is not
// integrated; a flag that cannot be written fails the move instead.
func TestRegression_ConflictingReparentMarksTheChildConflicting(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{ID: in.ID, Status: domain.WorkspaceStatusPRConflicts}, nil
	}
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: enginegit.ErrConflict, mergeBaseSha: "mb"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	out, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
	require.Len(t, synced, 1)
	assert.True(t, synced[0].HasConflicts, "git shows no conflict after the abort; the move must")
	assert.Equal(t, domain.WorkspaceStatusPRConflicts, out.Status)

	ws.SyncFn = func(_ context.Context, _ workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	_, err = uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom, "an unflagged conflicting move would be silent")
}

// TestRegression_ReparentKeepsCommitsAStaleForkPointWouldDrop: an imported
// branch recorded its own tip as its fork point, so the rebase replayed nothing
// and reset the branch onto its new parent. Homes already hold such rows.
func TestRegression_ReparentKeepsCommitsAStaleForkPointWouldDrop(t *testing.T) {
	h := newRealUsecase(t)
	ctx := context.Background()
	newParent := h.createChild(t, "feature/a", h.parentID, h.baseBranch)
	commitInWorktree(t, newParent.WorktreePath, "a.txt", "a\n", "a work")
	child := h.createChild(t, "feature/b", h.parentID, h.baseBranch)
	commitInWorktree(t, child.WorktreePath, "b.txt", "b\n", "b work")
	_, err := h.workspaces.UpdateForkPoint(ctx, child.ID, revParse(t, child.WorktreePath, "HEAD"))
	require.NoError(t, err)
	h.quiesce()

	_, err = h.uc.Reparent(ctx, child.ID, newParent.ID)
	require.NoError(t, err)

	assert.Equal(t, "b work", trimNewline(gitRun(t, child.WorktreePath, "log", "--format=%s", "feature/a..HEAD")),
		"the branch keeps its own commit, replayed onto the new parent")
	assert.Empty(t, gitRun(t, child.WorktreePath, "merge-base", "--is-ancestor", "feature/a", "HEAD"))
}

func TestReparent_AllowsLockedNewParent(t *testing.T) {
	// A locked (protected) branch is a valid re-parent target: it already adopts
	// children via create, so reparent must be consistent — the old "07 §4
	// new-parent-locked" block was incoherent and has been removed.
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", Status: domain.WorkspaceStatusLocked, WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
	assert.Equal(t, []string{"RevParse", "MergeBase", "RebaseOnto", "WorkingTreeSummary"}, g.ops())
}

func TestReparent_RebasesOntoNewTipAndUpdatesAggregate(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	var rID, rParent, rSha string
	ws.ReparentFn = func(_ context.Context, id, parentID, forkPointSha string, _ time.Time) (domain.Workspace, error) {
		rID, rParent, rSha = id, parentID, forkPointSha
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip", mergeBaseSha: "base"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
	// A clean rebase settles the move and resyncs the child's working-tree summary
	// against the new base, so a prior predicted-conflict status clears and the
	// diff stats reflect the new parent.
	assert.Equal(t, []string{"RevParse", "MergeBase", "RebaseOnto", "WorkingTreeSummary"}, g.ops())
	assert.Equal(t, []string{"/np", "HEAD"}, g.calls[0].args)
	assert.Equal(t, []string{"/cw", "ntip", "feat"}, g.calls[1].args,
		"a child with no parent row replays from its merge-base with the new tip")
	assert.Equal(t, []string{"/cw", "ntip", "base", "feat"}, g.calls[2].args,
		"the replay starts at the merge-base, never the recorded fork point")
	assert.Equal(t, []string{"/cw", "ntip"}, g.calls[3].args)
	assert.Equal(t, "c", rID)
	assert.Equal(t, "np", rParent)
	assert.Equal(t, "ntip", rSha)
}

func TestReparent_RebaseOntoError(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

// ForkPoint answers as a repo with no reflog would, so the fakes read merge-bases.
func (f *fakeGit) ForkPoint(_ context.Context, repoPath, upstream, branch string) (string, error) {
	f.record("ForkPoint", repoPath, upstream, branch)
	return "", nil
}

// stacked is P under the base with one commit, and C under P with its own.
func stacked(t *testing.T, h *realHarness) (domain.Workspace, domain.Workspace) {
	t.Helper()
	p := h.createChild(t, "feature/p", h.parentID, h.baseBranch)
	commitInWorktree(t, p.WorktreePath, "p.txt", "p\n", "p work")
	c := h.createChild(t, "feature/c", p.ID, p.Branch)
	commitInWorktree(t, c.WorktreePath, "c.txt", "c\n", "c work")
	return p, c
}

// amend rewrites P's commit with a different patch, as an amend or squash does.
func amend(t *testing.T, p domain.Workspace) {
	t.Helper()
	writeFile(t, filepath.Join(p.WorktreePath, "p.txt"), "p rewritten\n")
	gitRun(t, p.WorktreePath, "commit", "-a", "--amend", "-m", "p work, amended")
}

// movedOnto reparents c under a fresh sibling q of the base and returns c's
// commit subjects on top of q.
func movedOnto(t *testing.T, h *realHarness, c domain.Workspace) string {
	t.Helper()
	q := h.createChild(t, "feature/q", h.parentID, h.baseBranch)
	commitInWorktree(t, q.WorktreePath, "q.txt", "q\n", "q work")
	_, err := h.uc.Reparent(context.Background(), c.ID, q.ID)
	require.NoError(t, err)
	return trimNewline(gitRun(t, c.WorktreePath, "log", "--format=%s", "feature/q..HEAD"))
}

// TestRegression_ReparentReplaysOnlyCommitsTheOldParentLacks: the replay starts
// from the old parent, never the new tip, so P's commit stays behind.
func TestRegression_ReparentReplaysOnlyCommitsTheOldParentLacks(t *testing.T) {
	h := newRealUsecase(t)
	_, c := stacked(t, h)
	assert.Equal(t, "c work", movedOnto(t, h, c))
}

// TestRegression_ReparentRestacksOffAnAmendedParent: a merge-base with the
// rewritten parent predates the fork, so P's pre-amend commit was replayed too.
func TestRegression_ReparentRestacksOffAnAmendedParent(t *testing.T) {
	h := newRealUsecase(t)
	p, c := stacked(t, h)
	amend(t, p)
	assert.Equal(t, "c work", movedOnto(t, h, c))
}

// TestRegression_ReparentWithoutAReflogKeepsEveryCommit: with no reflog to find
// the fork, the merge-base replays more than C's own work but never less.
func TestRegression_ReparentWithoutAReflogKeepsEveryCommit(t *testing.T) {
	h := newRealUsecase(t)
	p, c := stacked(t, h)
	gitRun(t, h.repoPath, "config", "core.logAllRefUpdates", "false")
	amend(t, p)
	gitRun(t, h.repoPath, "reflog", "expire", "--expire=now", "--all")
	assert.Equal(t, "c work\np work", movedOnto(t, h, c))
}
