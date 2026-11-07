package hierarchy_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

func mergeWS(
	child domain.Workspace,
	parent domain.Workspace,
	children []domain.Workspace,
) *fakeWorkspace {
	list := append([]domain.Workspace{child, parent}, children...)
	return &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == child.ID {
				return child, nil
			}
			if id == parent.ID {
				return parent, nil
			}
			return domain.Workspace{}, fmt.Errorf("not found: %s", id)
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return list, nil
		},
	}
}

func TestMergeIntoParent_RejectsLockedParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", Status: domain.WorkspaceStatusLocked, WorktreePath: "/pw", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{}
	uc := hierarchy.New(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, hierarchy.ErrParentLocked)
	assert.Empty(t, g.calls)
}

func TestMergeIntoParent_RejectsRebaseForNonLeafChild(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	grandchild := domain.Workspace{ID: "gc", ParentID: "c"}
	g := &fakeGit{}
	uc := hierarchy.New(mergeWS(child, parent, []domain.Workspace{grandchild}), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyRebase)
	require.ErrorIs(t, err, hierarchy.ErrRebaseNonLeaf)
	assert.Empty(t, g.calls)
}

func TestMergeIntoParent_MergeStrategy_RunsInParentThenUpdatesForkPoint(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{revParseSha: "ptip"}
	ws := mergeWS(child, parent, nil)
	var updatedID, updatedSha string
	ws.UpdateForkPointFn = func(_ context.Context, id, sha string) (domain.Workspace, error) {
		updatedID, updatedSha = id, sha
		return domain.Workspace{}, nil
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err)
	assert.Equal(t, "ptip", res.ParentTipSha)
	assert.False(t, res.ConflictsPending)
	assert.Equal(t, []string{"MergeCommit", "RevParse", "WorkingTreeSummary", "WorkingTreeSummary"}, g.ops())
	assert.Equal(t, []string{"/pw", "feat"}, g.calls[0].args)
	assert.Equal(t, []string{"/pw", "HEAD"}, g.calls[1].args)
	assert.Equal(t, "c", updatedID)
	assert.Equal(t, "ptip", updatedSha)
}

func TestMergeIntoParent_ResyncsParentAndChildSummaries(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "cfork", Provisioning: domain.WorkspaceProvisioned}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", ForkPointSha: "pfork", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{revParseSha: "ptip", summaryAdded: 3, summaryDeleted: 1, summaryHasCommits: true}
	ws := mergeWS(child, parent, nil)
	ws.UpdateForkPointFn = func(_ context.Context, _, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, nil
	}
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{}, nil
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err)

	// Parent resyncs from its own fork point; child resyncs from the new tip.
	require.Len(t, synced, 2)
	assert.Equal(t, "p", synced[0].ID)
	assert.Equal(t, 3, synced[0].Added)
	assert.Equal(t, 1, synced[0].Deleted)
	assert.True(t, synced[0].HasCommits)
	assert.Equal(t, "c", synced[1].ID)
	assert.Equal(t, []string{"/pw", "pfork"}, g.calls[2].args, "parent summary uses parent fork point")
	assert.Equal(t, []string{"/cw", "ptip"}, g.calls[3].args, "child summary uses new tip")
}

// TestMergeIntoParent_ResyncSummaryError verifies that a WorkingTreeSummary
// failure during finalize does NOT fail MergeIntoParent — the git merge already
// committed durably and the read-model self-corrects on the next watcher event.
// UpdateForkPoint must still have been called (it is correctness-critical).
func TestMergeIntoParent_ResyncSummaryError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{revParseSha: "ptip", summaryErr: errBoom}
	ws := mergeWS(child, parent, nil)
	var forkUpdated bool
	ws.UpdateForkPointFn = func(_ context.Context, id, sha string) (domain.Workspace, error) {
		forkUpdated = true
		return domain.Workspace{}, nil
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err, "summary resync failure must not fail a durable merge")
	assert.Equal(t, "ptip", res.ParentTipSha)
	assert.False(t, res.ConflictsPending)
	assert.True(t, forkUpdated, "UpdateForkPoint must still be called")
}

func TestMergeIntoParent_SquashStrategy_RunsInParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{revParseSha: "ptip"}
	ws := mergeWS(child, parent, nil)
	ws.UpdateForkPointFn = func(_ context.Context, _, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, nil
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategySquash)
	require.NoError(t, err)
	assert.Equal(t, []string{"MergeSquash", "RevParse", "WorkingTreeSummary", "WorkingTreeSummary"}, g.ops())
	assert.Equal(t, "/pw", g.calls[0].args[0])
	assert.Equal(t, "feat", g.calls[0].args[1])
}

func TestMergeIntoParent_RebaseStrategy_RebasesChildThenFFMerges(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{revParseSha: "ptip"}
	ws := mergeWS(child, parent, nil)
	ws.UpdateForkPointFn = func(_ context.Context, _, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, nil
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyRebase)
	require.NoError(t, err)
	assert.Equal(t, []string{"RebaseThenFFMerge", "RevParse", "WorkingTreeSummary", "WorkingTreeSummary"}, g.ops())
	assert.Equal(t, []string{"/cw", "develop", "/pw", "feat"}, g.calls[0].args)
}

// TestMergeIntoParent_Conflict_SetsPRConflicts proves that a local merge
// conflict transitions the child to Status=pr-conflicts (via a
// SyncWorkingTreeState HasConflicts write) instead of recording a pending merge,
// AND aborts the in-progress merge in the PARENT worktree (where the plain
// merge runs) so neither worktree is left stuck (try-then-warn, H6/H7 guard).
func TestMergeIntoParent_Conflict_SetsPRConflicts(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{mergeErr: enginegit.ErrConflict}
	ws := mergeWS(child, parent, nil)
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{}, nil
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)
	assert.Empty(t, res.ParentTipSha)
	require.Len(t, synced, 1)
	assert.Equal(t, "c", synced[0].ID)
	assert.True(t, synced[0].HasConflicts)
	// The plain merge runs in the parent worktree, so the abort targets the parent.
	assert.Equal(t, []string{"MergeCommit", "OperationAbort"}, g.ops())
	assert.Equal(t, []string{"/pw"}, g.calls[1].args, "abort must run in the PARENT worktree")
}

// TestMergeIntoParent_RebaseConflict_SetsPRConflicts proves the rebase strategy
// aborts in the CHILD worktree (where RebaseThenFFMerge rebases the child) on a
// conflict, so the child is never left mid-rebase and the parent is untouched.
func TestMergeIntoParent_RebaseConflict_SetsPRConflicts(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{rebaseFFErr: enginegit.ErrConflict}
	ws := mergeWS(child, parent, nil)
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{}, nil
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyRebase)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)
	require.Len(t, synced, 1)
	assert.True(t, synced[0].HasConflicts)
	// The rebase runs in the child worktree, so the abort targets the child.
	assert.Equal(t, []string{"RebaseThenFFMerge", "OperationAbort"}, g.ops())
	assert.Equal(t, []string{"/cw"}, g.calls[1].args, "abort must run in the CHILD worktree")
}

// TestMergeIntoParent_SquashConflict_AbortsInParent proves the squash strategy
// aborts in the PARENT worktree (where MergeSquash runs) on a conflict. This is
// the core H6 guard: a conflicting squash merge must not brick the parent.
func TestMergeIntoParent_SquashConflict_AbortsInParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{squashErr: enginegit.ErrConflict}
	ws := mergeWS(child, parent, nil)
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{}, nil
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategySquash)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)
	require.Len(t, synced, 1)
	assert.True(t, synced[0].HasConflicts)
	assert.Equal(t, []string{"MergeSquash", "OperationAbort"}, g.ops())
	assert.Equal(t, []string{"/pw"}, g.calls[1].args, "squash abort must run in the PARENT worktree")
}

// TestMergeIntoParent_Conflict_AbortFailure_FlagsParentAndChild proves R7: if
// OperationAbort fails (the worktree stays mid-merge), the op is still best-effort
// (no error returned) but the STUCK worktree — the PARENT for the merge/squash
// strategies — is ALSO flagged pr-conflicts, so the brick is VISIBLE and
// recoverable instead of a silent merge-pending success.
func TestMergeIntoParent_Conflict_AbortFailure_FlagsParentAndChild(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{mergeErr: enginegit.ErrConflict, operationAbortErr: errBoom}
	ws := mergeWS(child, parent, nil)
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{}, nil
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)
	flagged := map[string]bool{}
	for _, s := range synced {
		if s.HasConflicts {
			flagged[s.ID] = true
		}
	}
	assert.True(t, flagged["p"], "the stuck PARENT worktree must be flagged pr-conflicts on abort failure (R7)")
	assert.True(t, flagged["c"], "the child must be flagged pr-conflicts")
}

// TestMergeIntoParent_Conflict_AbortSuccess_FlagsOnlyChild proves that when the
// abort SUCCEEDS (the normal case) the parent worktree is clean again, so only
// the child is flagged — the parent is NOT spuriously marked conflicted.
func TestMergeIntoParent_Conflict_AbortSuccess_FlagsOnlyChild(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{mergeErr: enginegit.ErrConflict} // abort succeeds
	ws := mergeWS(child, parent, nil)
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{}, nil
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)
	require.Len(t, synced, 1, "a clean abort flags only the child")
	assert.Equal(t, "c", synced[0].ID)
	assert.True(t, synced[0].HasConflicts)
}

func TestMergeIntoParent_NonConflictError_Propagates(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{mergeErr: errBoom}
	uc := hierarchy.New(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_GetChildError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_GetParentError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return domain.Workspace{ID: "c", ParentID: "p"}, nil
			}
			return domain.Workspace{}, errBoom
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_SetPRConflictsError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{mergeErr: enginegit.ErrConflict}
	ws := mergeWS(child, parent, nil)
	ws.SyncFn = func(_ context.Context, _ workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_UpdateForkPointError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{revParseSha: "ptip"}
	ws := mergeWS(child, parent, nil)
	ws.UpdateForkPointFn = func(_ context.Context, _, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_GuardListError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return child, nil
			}
			return parent, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, errBoom },
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyRebase)
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_RevParseError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{revParseErr: errBoom}
	uc := hierarchy.New(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

// TestMergeIntoParent_RejectsUnprovisionedParent proves a placeholder parent
// (locked + empty WorktreePath) is rejected before any git runs — no
// RevParse("", "HEAD"). It is ALSO rejected as locked; the empty-path guard is
// the explicit backstop the spec adds (§3.4/B2).
func TestMergeIntoParent_RejectsUnprovisionedParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	parent := domain.Workspace{ID: "p", Status: domain.WorkspaceStatusLocked, Provisioning: domain.WorkspacePlaceholder}
	g := &fakeGit{}
	uc := hierarchy.New(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.Error(t, err)
	assert.Empty(t, g.calls, "no git runs against an unprovisioned parent")
}

// A merge that would fold a leaf away while its chat works is refused before any
// git: nothing is left half-done. A non-leaf is never folded, so it may merge.
func TestGuardMergeFold_RefusesOnlyAWorkingLeaf(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Provisioning: domain.WorkspaceProvisioned}
	observer := func(working bool) *fakeChatObserver {
		return &fakeChatObserver{chats: []domain.Chat{{ID: "chat-1", WorkspaceID: "c"}}, working: map[string]bool{"chat-1": working}}
	}
	for name, tc := range map[string]struct {
		working bool
		kids    []domain.Workspace
		want    error
	}{
		"a working leaf":   {working: true, want: hierarchy.ErrMergeSourceWorking},
		"an idle leaf":     {},
		"a working parent": {working: true, kids: []domain.Workspace{{ID: "gc", ParentID: "c"}}},
	} {
		t.Run(name, func(t *testing.T) {
			g := &fakeGit{}
			uc := hierarchy.New(mergeWS(child, parent, tc.kids), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
			uc.SetChatObserver(observer(tc.working))

			err := uc.GuardMergeFold(context.Background(), "c")

			if tc.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
			assert.Empty(t, g.calls)
		})
	}
}
