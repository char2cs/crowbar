package tree_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Chat.Type only exists since #179 (2026-09-16). Every chat a production
// daemon minted before that has no /type patch in its event log and replays
// with Type == "" — jsonpatch never fills a field no event ever wrote. The
// lineage walks decide "is this a chat" with Type == ChatTypeChat alone
// (tree_snapshot.go isChat, walk.go ChatLineage, lineage/resolver.go), so a
// thread filed under such a chat is told it reads nothing.
func TestRegression_ChatLineageSkipsAPreTypeLegacyChat(t *testing.T) {
	chats := map[string]domain.Chat{
		"legacy": {ID: "legacy", WorkspaceID: "ws"},
		"child":  {ID: "child", Type: domain.ChatTypeChat, WorkspaceID: "ws", ParentID: "legacy"},
	}
	tr := treeFrom(chats)

	got := tree.ChatLineage(tr, chats, "child")

	if len(got) != 1 || got[0] != "legacy" {
		t.Fatalf("a legacy (Type==\"\") conversation is still a chat the thread reads; got %v", got)
	}
}
