package tree_test

import (
	"context"
	"testing"

	apptree "github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// stubListChats answers tree.Chats.ListChats from a fixed row set — the only
// method ResolveForkParent calls, so nothing else on the interface needs a
// body.
type stubListChats struct {
	tree.Chats
	rows []domain.Chat
	err  error
}

func (s stubListChats) ListChats(context.Context) ([]domain.Chat, error) {
	return s.rows, s.err
}

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

func TestResolveForkParent_ReadsEveryRowAndWalksLikeForkParentID(t *testing.T) {
	rows := []domain.Chat{
		{ID: "root", Type: domain.ChatTypeBranch, WorkspaceID: "ws-root"},
		{ID: "self", Type: domain.ChatTypeBranch, ParentID: "root", WorkspaceID: "ws-self"},
	}
	chats := stubListChats{rows: rows}

	got, ok, err := tree.ResolveForkParent(context.Background(), chats, "self")

	if err != nil || !ok || got != "ws-root" {
		t.Fatalf("want (ws-root, true, nil), got (%q, %v, %v)", got, ok, err)
	}
}

func TestResolveForkParent_NoAncestorAtAll_ReportsNotFound(t *testing.T) {
	rows := []domain.Chat{
		{ID: "root", Type: domain.ChatTypeChat},
	}
	chats := stubListChats{rows: rows}

	_, ok, err := tree.ResolveForkParent(context.Background(), chats, "root")

	if err != nil || ok {
		t.Fatalf("a row with no ancestor has no fork parent, got ok=%v err=%v", ok, err)
	}
}

func TestResolveForkParent_PropagatesTheListError(t *testing.T) {
	wantErr := context.Canceled
	chats := stubListChats{err: wantErr}

	_, _, err := tree.ResolveForkParent(context.Background(), chats, "any")

	if err != wantErr {
		t.Fatalf("want the list error propagated, got %v", err)
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
