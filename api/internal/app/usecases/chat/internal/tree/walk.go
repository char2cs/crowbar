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
//
// folders/nodes (2026-09-08 sidebar-placement-unification Task 8's own
// review fix round) fold in every reachable Folder+Node row first — see
// foldersReachableFromRoots — so a row whose walk passes through a folder
// ancestor still resolves correctly. Either port may be nil, degrading to
// the pre-Task-8, Chat-only walk.
func CwdWorkspaceIDs(
	ctx context.Context,
	folders Folders,
	nodes Nodes,
	rows []domain.Chat,
) map[string]string {
	if folderRows, err := foldersReachableFromRoots(ctx, folders, nodes, rows); err == nil {
		rows = append(rows, folderRows...)
	}
	treeNodes := make([]tree.Node, len(rows))
	byID := make(map[string]domain.Chat, len(rows))
	for i, row := range rows {
		treeNodes[i] = tree.Node{ID: row.ID, ParentID: row.ParentID, Order: row.Order, CreatedAt: row.CreatedAt}
		byID[row.ID] = row
	}
	forest := tree.New(treeNodes)
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
//
// folders/nodes fold in every reachable Folder+Node row (see
// foldersReachableFromRoots below) — 2026-09-08 sidebar-placement-
// unification Task 8's own review fix round: a folder is never a Chat row
// any more (home-scoped since Task 5, repo-scoped too since Task 8), so
// chats.ListChats' raw read no longer carries it at all, and a walk built
// from that read alone stops dead the moment it reaches a folder ancestor —
// even when a real fork parent/workspace sits one hop further up. Either
// port may be nil (a caller with no Folders/Nodes wired at all, e.g. a
// narrow test double) and the walk degrades to its old, Chat-only
// behaviour rather than failing outright.
func freshForest(
	ctx context.Context,
	chats Chats,
	folders Folders,
	nodes Nodes,
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
	if folderRows, ferr := foldersReachableFromRoots(ctx, folders, nodes, rows); ferr == nil {
		rows = append(rows, folderRows...)
	}
	treeNodes := make([]tree.Node, len(rows))
	byID := make(map[string]domain.Chat, len(rows))
	for i, row := range rows {
		treeNodes[i] = tree.Node{ID: row.ID, ParentID: row.ParentID, Order: row.Order, CreatedAt: row.CreatedAt}
		byID[row.ID] = row
	}
	return tree.New(treeNodes), byID, nil
}

// ResolveForkParent is ForkParentID over freshForest's log-corrected read of
// every row the daemon knows, for a caller with no snapshot of its own to walk
// (Promote, own_worktree.go).
func ResolveForkParent(
	ctx context.Context,
	chats Chats,
	folders Folders,
	nodes Nodes,
	rowID string,
) (string, bool, error) {
	t, byID, err := freshForest(ctx, chats, folders, nodes, rowID)
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
	folders Folders,
	nodes Nodes,
	rowID string,
) (string, bool, error) {
	t, byID, err := freshForest(ctx, chats, folders, nodes, rowID)
	if err != nil {
		return "", false, err
	}
	id, ok := CwdWorkspaceID(t, byID, rowID)
	return id, ok, nil
}

// foldersReachableFromRoots discovers every Folder+Node row filed somewhere
// between baseRows' own ids and the panel root, rendered as the same
// Chat-shaped view PlaceChat itself uses — mirrors chat package's own
// foldersReachableFrom (home_read.go) exactly (same BFS shape: seed the
// queue with "" plus every id baseRows already names, so a folder nested
// under an ordinary chat or a locked branch's own un-Noded owning row is
// still discovered), duplicated rather than imported because this package
// cannot import the outer chat package it is itself imported BY (no cross
// -package call here — see Folders/Nodes' own docs, home_ports.go, for why
// this package already carries these two ports for its own placement work).
//
// A nil folders or nodes port (a caller with neither wired) answers ("",
// nil) immediately — the walk simply gets no folder augmentation, exactly
// its pre-Task-8 behaviour.
func foldersReachableFromRoots(
	ctx context.Context,
	folders Folders,
	nodes Nodes,
	baseRows []domain.Chat,
) ([]domain.Chat, error) {
	if folders == nil || nodes == nil {
		return nil, nil
	}
	var found []domain.Chat
	queried := map[string]bool{}
	seenFolder := map[string]bool{}
	queue := make([]string, 0, len(baseRows)+1)
	queue = append(queue, "")
	for _, row := range baseRows {
		queue = append(queue, row.ID)
	}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		if queried[parent] {
			continue
		}
		queried[parent] = true
		children, err := nodes.ListByParent(ctx, parent)
		if err != nil {
			return nil, err
		}
		for _, n := range children {
			if n.Kind != domain.NodeKindFolder || seenFolder[n.ID] {
				continue
			}
			seenFolder[n.ID] = true
			f, ferr := folders.FindByKey(ctx, n.ID)
			if ferr != nil {
				return nil, ferr
			}
			if f == nil {
				continue
			}
			found = append(found, domain.Chat{
				ID: f.ID, Type: domain.ChatTypeFolder, RepoID: f.RepoID,
				Title: f.Name, ParentID: n.ParentID, Order: n.Order,
			})
			if !queried[n.ID] {
				queue = append(queue, n.ID)
			}
		}
	}
	return found, nil
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
