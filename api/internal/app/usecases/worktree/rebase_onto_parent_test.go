package worktree_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// rebaseWS builds a fakeWorkspace resolving exactly child and parent by id, for
// RebaseOntoParent tests (which never calls workspaces.List).
func rebaseWS(
	child domain.Workspace,
	parent domain.Workspace,
) *fakeWorkspace {
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
	}
}

func TestRebaseOntoParent_GetChildError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.RebaseOntoParent(context.Background(), "c")
	require.ErrorIs(t, err, errBoom)
}

// TestRebaseOntoParent_NoParent_ReturnsError proves a root workspace (no parent
// to rebase onto) is rejected with a plain error before any git or parent lookup.
func TestRebaseOntoParent_NoParent_ReturnsError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{ID: "c"}, nil // ParentID is empty
		},
	}
	g := &fakeGit{}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.RebaseOntoParent(context.Background(), "c")
	require.Error(t, err)
	assert.Empty(t, g.calls, "no git runs for a workspace with no parent")
}

func TestRebaseOntoParent_GetParentError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p"}
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return child, nil
			}
			return domain.Workspace{}, errBoom
		},
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.RebaseOntoParent(context.Background(), "c")
	require.ErrorIs(t, err, errBoom)
}

func TestRebaseOntoParent_ParentTipRevParseError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw"}
	g := &fakeGit{revParseErr: errBoom}
	uc := worktree.New(rebaseWS(child, parent), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.RebaseOntoParent(context.Background(), "c")
	require.ErrorIs(t, err, errBoom)
}

// TestRebaseOntoParent_CleanRebase_SettlesOntoParentTip proves a clean rebase
// (no conflict) integrates the child: it persists the PARENT'S TIP as the new
// fork point via the same settle path Reparent uses, rather than leaving
// anything predicted-conflicting.
func TestRebaseOntoParent_CleanRebase_SettlesOntoParentTip(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw"}
	ws := rebaseWS(child, parent)
	var rID, rParent, rSha string
	ws.ReparentFn = func(_ context.Context, id, parentID, forkPointSha string, _ time.Time) (domain.Workspace, error) {
		rID, rParent, rSha = id, parentID, forkPointSha
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ptip"}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.RebaseOntoParent(context.Background(), "c")
	require.NoError(t, err)
	assert.Equal(t, []string{"RevParse", "RebaseOnto", "WorkingTreeSummary"}, g.ops())
	assert.Equal(t, "c", rID)
	assert.Equal(t, "p", rParent)
	assert.Equal(t, "ptip", rSha, "the clean rebase settles onto the CURRENT parent tip")
}

func TestRebaseOntoParent_NonConflictRebaseError_Propagates(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw"}
	g := &fakeGit{revParseSha: "ptip", rebaseOnto: errBoom}
	uc := worktree.New(rebaseWS(child, parent), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.RebaseOntoParent(context.Background(), "c")
	require.ErrorIs(t, err, errBoom)
}

// TestRebaseOntoParent_ConflictPersistsForkPointAndFlagsConflicts proves the
// user-initiated "finish the move" DELIBERATELY differs from the automatic
// Reparent: on a conflict it does NOT abort — the rebase is left in progress so
// the user can resolve it with the standard conflict tooling — but the intended
// fork point (the parent's tip) is persisted up front and the child is flagged
// pr-conflicts so it reads correctly once resolved.
func TestRebaseOntoParent_ConflictPersistsForkPointAndFlagsConflicts(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw"}
	ws := rebaseWS(child, parent)
	var rID, rParent, rSha string
	ws.ReparentFn = func(_ context.Context, id, parentID, forkPointSha string, _ time.Time) (domain.Workspace, error) {
		rID, rParent, rSha = id, parentID, forkPointSha
		return domain.Workspace{ID: id}, nil
	}
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{ID: in.ID}, nil
	}
	g := &fakeGit{revParseSha: "ptip", rebaseOnto: enginegit.ErrConflict}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.RebaseOntoParent(context.Background(), "c")
	require.NoError(t, err)
	assert.Equal(t, "c", rID)
	assert.Equal(t, "p", rParent)
	assert.Equal(t, "ptip", rSha, "fork point is persisted as the parent tip even though the rebase is left mid-flight")
	require.Len(t, synced, 1)
	assert.Equal(t, "c", synced[0].ID)
	assert.True(t, synced[0].HasConflicts)
	assert.Equal(t, []string{"RevParse", "RebaseOnto"}, g.ops(),
		"unlike the automatic Reparent, a conflicting RebaseOntoParent must NEVER abort — no OperationAbort call")
}

func TestRebaseOntoParent_ConflictPersistError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw"}
	ws := rebaseWS(child, parent)
	ws.ReparentFn = func(_ context.Context, _, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	g := &fakeGit{revParseSha: "ptip", rebaseOnto: enginegit.ErrConflict}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.RebaseOntoParent(context.Background(), "c")
	require.ErrorIs(t, err, errBoom)
}

func TestRebaseOntoParent_ConflictSetConflictsError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw"}
	ws := rebaseWS(child, parent)
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	ws.SyncFn = func(_ context.Context, _ workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	g := &fakeGit{revParseSha: "ptip", rebaseOnto: enginegit.ErrConflict}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.RebaseOntoParent(context.Background(), "c")
	require.ErrorIs(t, err, errBoom)
}
