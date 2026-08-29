package tree_test

import (
	"testing"

	apptree "github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func treeFrom(
	chats map[string]domain.Chat,
) apptree.Tree {
	nodes := make([]apptree.Node, 0, len(chats))
	for _, c := range chats {
		nodes = append(nodes, apptree.Node{
			ID:        c.ID,
			ParentID:  c.ParentID,
			Order:     c.Order,
			CreatedAt: c.CreatedAt,
		})
	}
	return apptree.New(nodes)
}

func TestCwdWorkspaceID_SkipsUnprovisionedAncestor(t *testing.T) {
	chats := map[string]domain.Chat{
		"root":    {ID: "root", Type: domain.ChatTypeBranch, WorkspaceID: "ws-root"},
		"folder":  {ID: "folder", Type: domain.ChatTypeFolder, ParentID: "root"},
		"blocked": {ID: "blocked", Type: domain.ChatTypeBranch, ParentID: "folder", WorkspaceID: ""},
		"leaf":    {ID: "leaf", Type: domain.ChatTypeChat, ParentID: "blocked"},
	}
	tr := treeFrom(chats)

	got, ok := tree.CwdWorkspaceID(tr, chats, "leaf")

	if !ok || got != "ws-root" {
		t.Fatalf("want ws-root (walk past the unprovisioned blocked row), got %q ok=%v", got, ok)
	}
}

func TestForkParentID_ExcludesSelf(t *testing.T) {
	chats := map[string]domain.Chat{
		"root": {ID: "root", Type: domain.ChatTypeBranch, WorkspaceID: "ws-root"},
		"self": {ID: "self", Type: domain.ChatTypeBranch, ParentID: "root", WorkspaceID: "ws-self"},
	}
	tr := treeFrom(chats)

	got, ok := tree.ForkParentID(tr, chats, "self")

	if !ok || got != "ws-root" {
		t.Fatalf("fork parent must exclude self's own workspace, want ws-root, got %q ok=%v", got, ok)
	}
}

func TestChatLineage_StopsAtNonChatRow(t *testing.T) {
	chats := map[string]domain.Chat{
		"folder": {ID: "folder", Type: domain.ChatTypeFolder},
		"parent": {ID: "parent", Type: domain.ChatTypeChat, ParentID: "folder"},
		"child":  {ID: "child", Type: domain.ChatTypeChat, ParentID: "parent"},
	}
	tr := treeFrom(chats)

	got := tree.ChatLineage(tr, chats, "child")

	if len(got) != 1 || got[0] != "parent" {
		t.Fatalf("lineage must only include Type==chat ancestors, got %v", got)
	}
}
