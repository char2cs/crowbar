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
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// TestMergeIntoParent_Conflict_AbortFailure_ParentFlagAlsoFails proves that
// when BOTH the post-conflict abort AND the follow-up attempt to flag the
// stuck parent fail, the merge is still reported as pending conflicts (never a
// hard failure) and the CHILD is still flagged regardless.
func TestMergeIntoParent_Conflict_AbortFailure_ParentFlagAlsoFails(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{mergeErr: enginegit.ErrConflict, operationAbortErr: errBoom}
	ws := mergeWS(child, parent, nil)
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		if in.ID == parent.ID {
			return domain.Workspace{}, errBoom2
		}
		return domain.Workspace{}, nil
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err, "a failed attempt to flag the stuck parent must still be best-effort")
	assert.True(t, res.ConflictsPending)
	require.Len(t, synced, 2, "both the parent-flag attempt and the child flag must have run")
	var childFlagged bool
	for _, s := range synced {
		if s.ID == "c" {
			childFlagged = s.HasConflicts
		}
	}
	assert.True(t, childFlagged, "the child must still be flagged even though flagging the stuck parent failed")
}

// TestMergeIntoParent_ResyncSummary_SyncWriteFails_IsBestEffort proves the
// resync's own aggregate-WRITE failure (distinct from a WorkingTreeSummary
// READ failure, covered elsewhere) is best-effort: the merge itself already
// committed durably, so the merge result must still report success.
func TestMergeIntoParent_ResyncSummary_SyncWriteFails_IsBestEffort(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{revParseSha: "ptip"}
	ws := mergeWS(child, parent, nil)
	ws.UpdateForkPointFn = func(_ context.Context, _, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, nil
	}
	ws.SyncFn = func(_ context.Context, _ workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err, "a resync SYNC-write failure must not fail a durable merge")
	assert.Equal(t, "ptip", res.ParentTipSha)
}
