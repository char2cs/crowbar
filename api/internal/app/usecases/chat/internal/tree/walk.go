package tree

import (
	"context"

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

// CwdWorkspaceIDs answers CwdWorkspaceID for EVERY row in one pass, keyed by
// row id, omitting the rows whose walk resolves nothing — a true orphan, whose
// whole ancestry owns no workspace.
//
// It exists so a caller asking the question of a whole list builds the forest
// once instead of once per row. The walk itself is the same one; nothing here
// re-implements it.
func CwdWorkspaceIDs(
	rows []domain.Chat,
) map[string]string {
	nodes := make([]tree.Node, len(rows))
	byID := make(map[string]domain.Chat, len(rows))
	for i, row := range rows {
		nodes[i] = tree.Node{ID: row.ID, ParentID: row.ParentID, Order: row.Order, CreatedAt: row.CreatedAt}
		byID[row.ID] = row
	}
	forest := tree.New(nodes)
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		if id, ok := CwdWorkspaceID(forest, byID, row.ID); ok {
			out[row.ID] = id
		}
	}
	return out
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

// freshForest reads every row the daemon knows and builds the tree.Tree plus
// id->row map every Resolve* walk below needs, with rowID's OWN row corrected
// from LoadChat's log fold first — exactly as globalSnapshotAround/
// workspaceSnapshotAround already do for a placement plan (see corrected's own
// doc, plan.go).
//
// rowID may have been minted AND placed microseconds ago in this SAME call —
// CreateChat's ownWorktree path mints, places, then immediately resolves the
// new row's fork parent, and Promote's caller resolved a chat some earlier,
// already-settled request placed — so ListChats' asynchronous projection can
// still be serving rowID's state from BEFORE that placement, or omit it
// entirely. Every row ABOVE rowID came from an earlier, unrelated write and is
// read from the list safely; only rowID itself needs the log-true answer.
//
// This is the one read+build step every Resolve* walk below needs and no
// caller with its own in-progress plan does — PlaceChat and friends already
// hold a snapshot to walk instead.
func freshForest(
	ctx context.Context,
	chats Chats,
	rowID string,
) (tree.Tree, map[string]domain.Chat, error) {
	subject, err := chats.LoadChat(ctx, rowID)
	if err != nil {
		return nil, nil, err
	}
	rows, err := chats.ListChats(ctx)
	if err != nil {
		return nil, nil, err
	}
	rows = corrected(rows, subject)
	nodes := make([]tree.Node, len(rows))
	byID := make(map[string]domain.Chat, len(rows))
	for i, row := range rows {
		nodes[i] = tree.Node{ID: row.ID, ParentID: row.ParentID, Order: row.Order, CreatedAt: row.CreatedAt}
		byID[row.ID] = row
	}
	return tree.New(nodes), byID, nil
}

// ResolveForkParent is ForkParentID over freshForest's log-corrected read of
// every row the daemon knows, for a caller with no snapshot of its own to walk
// (Promote, own_worktree.go).
func ResolveForkParent(
	ctx context.Context,
	chats Chats,
	rowID string,
) (string, bool, error) {
	t, byID, err := freshForest(ctx, chats, rowID)
	if err != nil {
		return "", false, err
	}
	id, ok := ForkParentID(t, byID, rowID)
	return id, ok, nil
}

// ResolveCwdWorkspaceID is CwdWorkspaceID over freshForest's log-corrected
// read — the spawn path's own read+build step, alongside ResolveForkParent's.
func ResolveCwdWorkspaceID(
	ctx context.Context,
	chats Chats,
	rowID string,
) (string, bool, error) {
	t, byID, err := freshForest(ctx, chats, rowID)
	if err != nil {
		return "", false, err
	}
	id, ok := CwdWorkspaceID(t, byID, rowID)
	return id, ok, nil
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
