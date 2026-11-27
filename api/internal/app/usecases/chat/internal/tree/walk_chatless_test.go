package tree_test

import (
	"context"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

func TestRegression_ResolveForkParent_ParentIsAChatlessWorkspaceAnchorNode(t *testing.T) {
	rows := []domain.Chat{
		{ID: "self", Type: domain.ChatTypeChat, ParentID: "ws-main"},
	}
	chats := stubListChats{rows: rows}
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []dtree.Node{
		{ID: "ws-main", Kind: dtree.KindWorkspace, ParentID: "", Order: 0},
	}

	got, ok, err := tree.ResolveForkParent(context.Background(), chats, folders, nodes, nil, "self")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || got != "ws-main" {
		t.Fatalf("a chat placed under a workspace's own Node row must fork from that workspace; got (%q, %v)", got, ok)
	}
}

type stubRepoRoots map[string]string

func (s stubRepoRoots) DefaultWorkspaceOf(_ context.Context, repoID string) (string, error) {
	return s[repoID], nil
}

func TestRegression_ForkUnderRepoRootFolderForksTheDefaultWorkspace(t *testing.T) {
	rows := []domain.Chat{
		{ID: "owner", Type: domain.ChatTypeChat, WorkspaceID: "ws-main"},
		{ID: "self", Type: domain.ChatTypeChat, ParentID: "folder"},
	}
	chats := stubListChats{rows: rows}
	folders := mocks.NewFolderStore()
	folders.Saved = []dtree.Folder{{ID: "folder", Name: "F", RepoID: "repo-1"}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []dtree.Node{
		{ID: "folder", Kind: dtree.KindFolder, ParentID: ""},
		{ID: "ws-main", Kind: dtree.KindWorkspace, ParentID: ""},
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
