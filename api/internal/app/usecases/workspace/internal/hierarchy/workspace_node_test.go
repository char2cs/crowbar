package hierarchy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// fakeNodeCreator stands in for the Node surface WithNodes wires: it records
// every Create call so a test can prove a workspace's own Node row was minted
// (or was not, when the create itself failed) with no separate backfill step.
type fakeNodeCreator struct {
	created []domain.Node
	err     error
}

func (f *fakeNodeCreator) Create(
	_ context.Context,
	id string,
	kind domain.NodeKind,
	parentID string,
	order int,
) (domain.Node, error) {
	if f.err != nil {
		return domain.Node{}, f.err
	}
	n := domain.Node{ID: id, Kind: kind, ParentID: parentID, Order: order}
	f.created = append(f.created, n)
	return n, nil
}

func (f *fakeNodeCreator) idFor(id string) (domain.Node, bool) {
	for _, n := range f.created {
		if n.ID == id {
			return n, true
		}
	}
	return domain.Node{}, false
}

// TestCreateChild_NoRepoPath_MintsWorkspaceNode proves the path-less/no-worktree
// CreateChild branch mints the new workspace's own Node{Kind:workspace} row
// immediately, with no separate backfill or lazy-mint step (2026-09-08
// sidebar-placement-unification Task 7).
func TestCreateChild_NoRepoPath_MintsWorkspaceNode(t *testing.T) {
	g := &fakeGit{}
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	nodes := &fakeNodeCreator{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(), hierarchy.WithNodes(nodes))

	out, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", Branch: "feature/x", ParentID: "w-parent",
	})
	require.NoError(t, err)

	n, ok := nodes.idFor(out.ID)
	require.True(t, ok, "the new workspace must have its own Node row the instant it is created")
	assert.Equal(t, domain.NodeKindWorkspace, n.Kind)
}

// TestCreateChild_WorktreeBacked_MintsWorkspaceNode proves the real,
// worktree-creating CreateChild path (a live fork) mints the same invariant.
func TestCreateChild_WorktreeBacked_MintsWorkspaceNode(t *testing.T) {
	g := &fakeGit{addStartSha: "sha123"}
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	nodes := &fakeNodeCreator{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(), hierarchy.WithNodes(nodes))

	out, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)

	n, ok := nodes.idFor(out.ID)
	require.True(t, ok, "a live fork's workspace must have its own Node row the instant it is created")
	assert.Equal(t, domain.NodeKindWorkspace, n.Kind)
}

// TestCreateChild_AdoptMainWorktree_MintsWorkspaceNode proves the repo-home
// adoption path (a locked-branch/repo-home workspace-kind row) mints its own
// Node row too, unconditionally — the same invariant as an ordinary fork.
func TestCreateChild_AdoptMainWorktree_MintsWorkspaceNode(t *testing.T) {
	g := &fakeGit{revParseSha: "headsha"}
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, nil },
	}
	nodes := &fakeNodeCreator{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(), hierarchy.WithNodes(nodes))

	out, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "main",
		ParentID:     "",
		ParentBranch: "main",
	})
	require.NoError(t, err)

	n, ok := nodes.idFor(out.ID)
	require.True(t, ok, "the adopted repo-home workspace must have its own Node row the instant it is created")
	assert.Equal(t, domain.NodeKindWorkspace, n.Kind)
}

// TestCreateFromImport_Placeholder_MintsWorkspaceNode proves the batch-import
// placeholder fallback (a branch that could not be materialised) also mints
// its own Node row — the fourth and last hierarchy-package creation path.
func TestCreateFromImport_Placeholder_MintsWorkspaceNode(t *testing.T) {
	var placeholderID string
	inner := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			placeholderID = in.ID
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	g := &fakeGit{addErr: errBoom} // WorktreeAddBranch fails: a plain git failure, no holder involved
	nodes := &fakeNodeCreator{}
	uc := hierarchy.New(inner, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(), hierarchy.WithNodes(nodes))
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: []string{"feat/x"},
	})
	require.NoError(t, err, "a free branch that fails to materialise still yields a placeholder row")
	require.NotEmpty(t, placeholderID)

	n, ok := nodes.idFor(placeholderID)
	require.True(t, ok, "the placeholder workspace must have its own Node row the instant it is created")
	assert.Equal(t, domain.NodeKindWorkspace, n.Kind)
}

// TestCreateChild_NoWithNodesOption_UsesNoOpDefaultAndStillSucceeds proves
// hierarchy.New defaults u.nodes to a trivially-succeeding no-op when no
// WithNodes option is given at all (the shape all ~180 pre-existing
// hierarchy.New(...) test call sites use), so those tests keep passing
// unchanged even though u.nodes is never nil.
func TestCreateChild_NoWithNodesOption_UsesNoOpDefaultAndStillSucceeds(t *testing.T) {
	g := &fakeGit{}
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	out, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", Branch: "feature/x", ParentID: "w-parent",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.ID)
}

// ── Node-mint failure ⇒ full rollback (2026-09-08 sidebar-placement-unification
// Task 7 review fix) ─────────────────────────────────────────────────────────
//
// The mint used to be best-effort: log a warning, let the create stand. That
// was wrong even in a perfectly healthy, fully-wired daemon — u.nodes.Create
// runs AFTER a potentially-slow git worktree operation, so a client
// disconnect/timeout cancelling ctx in that window fails it as a direct
// consequence, not a misconfiguration, and (unlike the old EnsureOwningChat)
// there is no boot-time backfill to catch the straggler. Every path below now
// proves the WHOLE create rolls back instead: no orphaned Workspace row
// survives with no Node row of its own.

// TestCreateChild_NoRepoPath_NodeMintFails_RollsBackWorkspaceRow covers the
// path-less/no-worktree branch: nothing on disk to unwind, but the just-created
// workspace row must still be taken back out.
func TestCreateChild_NoRepoPath_NodeMintFails_RollsBackWorkspaceRow(t *testing.T) {
	g := &fakeGit{}
	var createdID string
	var deletedIDs []string
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			createdID = in.ID
			return domain.Workspace{ID: in.ID}, nil
		},
		DeleteFn: func(_ context.Context, id string) error {
			deletedIDs = append(deletedIDs, id)
			return nil
		},
	}
	nodes := &fakeNodeCreator{err: errors.New("node create boom")}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(), hierarchy.WithNodes(nodes))

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", Branch: "feature/x", ParentID: "w-parent",
	})

	require.Error(t, err, "a failed node mint must now fail the whole create")
	require.NotEmpty(t, createdID)
	assert.Equal(t, []string{createdID}, deletedIDs,
		"the workspace whose own node row could not be minted must be taken back out")
}

// TestCreateChild_WorktreeBacked_NodeMintFails_RollsBackWorktreeBranchAndRow
// is the critical regression test: a live fork (very likely the most common
// workspace-creation path in the app) whose worktree and branch were already
// created on disk must have ALL of it — worktree, branch, and the workspace
// row itself — rolled back when its own Node row fails to mint, exactly as a
// failed workspaces.Create already does, not just logged and left standing.
func TestCreateChild_WorktreeBacked_NodeMintFails_RollsBackWorktreeBranchAndRow(t *testing.T) {
	g := &fakeGit{addStartSha: "sha123"}
	var createdID string
	var deletedIDs []string
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			createdID = in.ID
			return domain.Workspace{ID: in.ID}, nil
		},
		DeleteFn: func(_ context.Context, id string) error {
			deletedIDs = append(deletedIDs, id)
			return nil
		},
	}
	nodes := &fakeNodeCreator{err: errors.New("node create boom")}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(), hierarchy.WithNodes(nodes))

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})

	require.Error(t, err, "a failed node mint must now fail the whole create")
	require.NotEmpty(t, createdID)
	assert.Equal(t, []string{createdID}, deletedIDs,
		"the workspace whose own node row could not be minted must be taken back out")
	ops := g.ops()
	assert.Contains(t, ops, "WorktreeRemove", "the orphaned worktree must be removed too")
	assert.Contains(t, ops, "ForceDeleteBranch", "the orphaned branch must be deleted too")
}

// TestCreateChild_AdoptMainWorktree_NodeMintFails_RollsBackWorkspaceRow covers
// the repo-home adoption path: no NEW worktree/branch was created (it adopts
// the repo's existing checkout in place), but the workspace row itself must
// still be rolled back.
func TestCreateChild_AdoptMainWorktree_NodeMintFails_RollsBackWorkspaceRow(t *testing.T) {
	g := &fakeGit{revParseSha: "headsha"}
	var createdID string
	var deletedIDs []string
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			createdID = in.ID
			return domain.Workspace{ID: in.ID}, nil
		},
		DeleteFn: func(_ context.Context, id string) error {
			deletedIDs = append(deletedIDs, id)
			return nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, nil },
	}
	nodes := &fakeNodeCreator{err: errors.New("node create boom")}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(), hierarchy.WithNodes(nodes))

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "main",
		ParentID:     "",
		ParentBranch: "main",
	})

	require.Error(t, err, "a failed node mint must now fail the whole create")
	require.NotEmpty(t, createdID)
	assert.Equal(t, []string{createdID}, deletedIDs,
		"the adopted workspace whose own node row could not be minted must be taken back out")
}

// TestCreateFromImport_Placeholder_NodeMintFails_RollsBackWorkspaceAndChat
// covers the fourth hierarchy-package path: a placeholder whose own Node row
// fails to mint must take BOTH the workspace row and the chat minted to own
// it back out, via the same discardUnownedWorkspace path a failed
// AttachOwningWorkspace already uses.
func TestCreateFromImport_Placeholder_NodeMintFails_RollsBackWorkspaceAndChat(t *testing.T) {
	var createdID string
	var deletedIDs []string
	inner := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			createdID = in.ID
			return domain.Workspace{ID: in.ID}, nil
		},
		DeleteFn: func(_ context.Context, id string) error {
			deletedIDs = append(deletedIDs, id)
			return nil
		},
	}
	g := &fakeGit{addErr: errBoom} // WorktreeAddBranch fails: a plain git failure, no holder involved
	nodes := &fakeNodeCreator{err: errors.New("node create boom")}
	uc := hierarchy.New(inner, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(), hierarchy.WithNodes(nodes))
	chats := withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: []string{"feat/x"},
	})

	require.Error(t, err, "a placeholder whose own node row could not be minted produces no row at all")
	require.NotEmpty(t, createdID)
	assert.Equal(t, []string{createdID}, deletedIDs,
		"the placeholder whose own node row could not be minted must be taken back out")
	assert.NotEmpty(t, chats.discards(), "the chat minted for the placeholder goes too")
}
