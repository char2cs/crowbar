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
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type fakeNodeCreator struct {
	created []tree.Node
	err     error
}

func (f *fakeNodeCreator) Create(
	_ context.Context,
	id string,
	kind tree.Kind,
	parentID string,
	order int,
) (tree.Node, error) {
	if f.err != nil {
		return tree.Node{}, f.err
	}
	n := tree.Node{ID: id, Kind: kind, ParentID: parentID, Order: order}
	f.created = append(f.created, n)
	return n, nil
}

func (f *fakeNodeCreator) idFor(id string) (tree.Node, bool) {
	for _, n := range f.created {
		if n.ID == id {
			return n, true
		}
	}
	return tree.Node{}, false
}

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
	assert.Equal(t, tree.KindWorkspace, n.Kind)
}

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
	assert.Equal(t, tree.KindWorkspace, n.Kind)
}

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
	assert.Equal(t, tree.KindWorkspace, n.Kind)
}

func TestCreateFromImport_Placeholder_MintsWorkspaceNode(t *testing.T) {
	var placeholderID string
	inner := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			placeholderID = in.ID
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	g := &fakeGit{addErr: errBoom}
	nodes := &fakeNodeCreator{}
	uc := hierarchy.New(inner, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(), hierarchy.WithNodes(nodes))
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: branches("feat/x"),
	}, &outcomes{})
	require.NoError(t, err, "a free branch that fails to materialise still yields a placeholder row")
	require.NotEmpty(t, placeholderID)

	n, ok := nodes.idFor(placeholderID)
	require.True(t, ok, "the placeholder workspace must have its own Node row the instant it is created")
	assert.Equal(t, tree.KindWorkspace, n.Kind)
}

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
	g := &fakeGit{addErr: errBoom}
	nodes := &fakeNodeCreator{err: errors.New("node create boom")}
	uc := hierarchy.New(inner, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(), hierarchy.WithNodes(nodes))
	chats := withOwningChats(uc)

	got := &outcomes{}
	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: branches("feat/x"),
	}, got)

	require.NoError(t, err)
	require.ErrorContains(t, got.of(t, "feat/x").Err, "node create boom",
		"a placeholder whose own node row could not be minted is reported with why")
	require.NotEmpty(t, createdID)
	assert.Equal(t, []string{createdID}, deletedIDs,
		"the placeholder whose own node row could not be minted must be taken back out")
	assert.NotEmpty(t, chats.discards(), "the chat minted for the placeholder goes too")
}
