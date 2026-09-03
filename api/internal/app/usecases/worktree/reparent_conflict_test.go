package worktree_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
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
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	ws := reparentWS(child, newParent, nil)
	var rID, rParent, rSha string
	ws.ReparentFn = func(_ context.Context, id, parentID, forkPointSha string, _ time.Time) (domain.Workspace, error) {
		rID, rParent, rSha = id, parentID, forkPointSha
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: enginegit.ErrConflict, mergeBaseSha: "mergebase-sha"}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
	assert.Equal(t, []string{"RevParse", "RebaseOnto", "OperationAbort", "MergeBase", "WorkingTreeSummary"}, g.ops())
	assert.Equal(t, []string{"/cw"}, g.calls[2].args, "the abort must run in the CHILD worktree")
	assert.Equal(t, []string{"/cw", "ntip", "feat"}, g.calls[3].args, "merge-base of the new parent tip and the branch")
	assert.Equal(t, "c", rID)
	assert.Equal(t, "np", rParent)
	assert.Equal(t, "mergebase-sha", rSha, "the move lands at the MERGE-BASE, not the new parent's raw tip")
}

// TestReparent_ConflictAbortFails proves that when the post-conflict abort
// itself fails, Reparent surfaces the error and never persists the move — a
// stuck mid-rebase worktree must not be silently reported as reparented.
func TestReparent_ConflictAbortFails(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	ws := reparentWS(child, newParent, nil)
	persisted := false
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		persisted = true
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: enginegit.ErrConflict, operationAbortErr: errBoom}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
	assert.False(t, persisted, "a failed abort must never persist a partial move")
	assert.NotContains(t, g.ops(), "MergeBase", "no merge-base attempt after a failed abort")
}

// TestReparent_ConflictMergeBaseFails proves a merge-base resolution failure
// (after a successful abort) surfaces cleanly and never persists the move.
func TestReparent_ConflictMergeBaseFails(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	ws := reparentWS(child, newParent, nil)
	persisted := false
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		persisted = true
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: enginegit.ErrConflict, mergeBaseErr: errBoom}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
	assert.False(t, persisted, "no move is persisted when the merge-base cannot be resolved")
}

// TestReparent_PersistError proves a clean rebase whose PERSIST step fails
// (the aggregate write) surfaces the error, even though the git-side rebase
// already succeeded.
func TestReparent_PersistError(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, _, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

// TestReparent_ResyncSummaryError_IsBestEffort proves that when the persist
// succeeds but the follow-up working-tree-summary SYNC write fails (distinct
// from a WorkingTreeSummary READ failure, which a separate test already
// covers), Reparent still reports success — the resync is best-effort and the
// read model self-corrects on the next watcher event.
func TestReparent_ResyncSummaryError_IsBestEffort(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	ws.SyncFn = func(_ context.Context, _ workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	out, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err, "a resync sync-write failure must not fail the reparent")
	assert.Equal(t, "c", out.ID)
}
