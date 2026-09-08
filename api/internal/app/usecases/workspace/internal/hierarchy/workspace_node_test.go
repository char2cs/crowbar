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

// TestCreateChild_NoNodesWired_StillSucceeds proves the Node mint is
// best-effort in this package (unlike the project package's harder
// ErrNoNodesWired refusal): nothing yet reads a workspace's own Node row, so a
// usecase built with no WithNodes option at all must not fail the create.
func TestCreateChild_NoNodesWired_StillSucceeds(t *testing.T) {
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

// TestCreateChild_NodeCreateFails_StillSucceeds proves a Node.Create failure
// never fails the workspace create it rode in on — the lock has already been
// committed and stands, mirroring EnsureOwningChat's own never-fail-the-write
// contract for a secondary reconciliation write.
func TestCreateChild_NodeCreateFails_StillSucceeds(t *testing.T) {
	g := &fakeGit{}
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	nodes := &fakeNodeCreator{err: errors.New("node create boom")}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(), hierarchy.WithNodes(nodes))

	out, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", Branch: "feature/x", ParentID: "w-parent",
	})
	require.NoError(t, err, "a failed node mint must not fail the workspace create")
	assert.NotEmpty(t, out.ID)
}
