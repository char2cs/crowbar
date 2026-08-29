package tree

import (
	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree/internal/lineage"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// CwdWorkspaceID answers where rowID's CLI runs: the WorkspaceID of the
// nearest ancestor-or-self that carries one. An unprovisioned row along the
// way — WorkspaceID set to "" while its own workspace resolves — is skipped
// rather than stopped on, so a blocked row still resolves a cwd from whatever
// it hangs off.
//
// It takes t and chats as plain values rather than reading a store because it
// is pure by construction: the same walk answers a read and a move still
// being planned in memory, and a store re-read would only ever serve one of
// the two.
func CwdWorkspaceID(
	t tree.Tree,
	chats map[string]domain.Chat,
	rowID string,
) (string, bool) {
	seen := map[string]bool{}
	for id := rowID; id != "" && !seen[id]; {
		seen[id] = true
		if c, ok := chats[id]; ok && c.WorkspaceID != "" {
			return c.WorkspaceID, true
		}
		node, ok := t.Node(id)
		if !ok {
			break
		}
		id = node.ParentID
	}
	return "", false
}

// ForkParentID answers what a new branch under rowID forks from: the same
// walk as CwdWorkspaceID, started one row up so rowID's own WorkspaceID is
// never mistaken for what forks from it.
func ForkParentID(
	t tree.Tree,
	chats map[string]domain.Chat,
	rowID string,
) (string, bool) {
	node, ok := t.Node(rowID)
	if !ok {
		return "", false
	}
	return CwdWorkspaceID(t, chats, node.ParentID)
}

// ChatLineage answers what rowID reads: its CHAT ancestors, nearest first,
// stepping through every folder between them transparently. It is the same
// traversal internal/lineage.Walk already runs for the tree's own placement
// reads, resolved here over the generic plan and a chat map instead of the
// package's private snapshot.
func ChatLineage(
	t tree.Tree,
	chats map[string]domain.Chat,
	rowID string,
) []string {
	return lineage.Walk(
		rowID,
		func(id string) string {
			node, _ := t.Node(id)
			return node.ParentID
		},
		func(id string) bool {
			return chats[id].Type == domain.ChatTypeChat
		},
	)
}
