package tree_test

import (
	"context"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// A fork created under a CHATLESS workspace row: the frontend's
// handleCreate('workspace') resolves no owningChatId for it and sends the
// workspace's own id as parentId (space-content-actions.ts placementParentId
// -> resolveHomeOwnerId -> homeId). checkNewChatParent accepts that id
// (workspaceAnchorType via nodes.GetNode), placeChat files the new chat
// under it, and then SpawnChatWithOwnWorktree's ResolveForkParent walks a
// forest built from chats + folder Nodes only -- the workspace anchor Node
// is never in it, so the walk dead-ends and the create fails with
// ErrNoForkParent even though the parent is a real, provisioned workspace.
func TestRegression_ResolveForkParent_ParentIsAChatlessWorkspaceAnchorNode(t *testing.T) {
	rows := []domain.Chat{
		{ID: "self", Type: domain.ChatTypeChat, ParentID: "ws-main"},
	}
	chats := stubListChats{rows: rows}
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []domain.Node{
		{ID: "ws-main", Kind: domain.NodeKindWorkspace, ParentID: "", Order: 0},
	}

	got, ok, err := tree.ResolveForkParent(context.Background(), chats, folders, nodes, nil, "self")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || got != "ws-main" {
		t.Fatalf("a chat placed under a workspace's own Node row must fork from that workspace; got (%q, %v)", got, ok)
	}
}

// stubRepoRoots answers tree.RepoRoots from a fixed repo->default-workspace map.
type stubRepoRoots map[string]string

func (s stubRepoRoots) DefaultWorkspaceOf(_ context.Context, repoID string) (string, error) {
	return s[repoID], nil
}

// A repo-ROOT folder's Node sits at ParentID "" with no workspace-owning row
// above it (row-actions.ts root-normalises a folder started on the repo
// header), so a fork filed inside it walked to "" and was refused. The folder
// applies its parent's logic: its parent is the repo header, i.e. the repo's
// default checkout.
func TestRegression_ForkUnderRepoRootFolderForksTheDefaultWorkspace(t *testing.T) {
	rows := []domain.Chat{
		{ID: "owner", Type: domain.ChatTypeChat, WorkspaceID: "ws-main"},
		{ID: "self", Type: domain.ChatTypeChat, ParentID: "folder"},
	}
	chats := stubListChats{rows: rows}
	folders := mocks.NewFolderStore()
	folders.Saved = []domain.Folder{{ID: "folder", Name: "F", RepoID: "repo-1"}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []domain.Node{
		{ID: "folder", Kind: domain.NodeKindFolder, ParentID: ""},
		{ID: "ws-main", Kind: domain.NodeKindWorkspace, ParentID: ""},
	}
	roots := stubRepoRoots{"repo-1": "ws-main"}

	got, ok, err := tree.ResolveForkParent(context.Background(), chats, folders, nodes, roots, "self")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || got != "ws-main" {
		t.Fatalf("a fork inside a repo-root folder must fork the repo's default checkout; got (%q, %v)", got, ok)
	}

	cwd, ok, err := tree.ResolveCwdWorkspaceID(context.Background(), chats, folders, nodes, roots, "self")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || cwd != "ws-main" {
		t.Fatalf("a bubble inside a repo-root folder runs in the repo's default checkout; got (%q, %v)", cwd, ok)
	}
}
