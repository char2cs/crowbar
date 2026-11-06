package tree

import (
	"context"
	"errors"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

// RepoRoots answers a repo's default checkout: where a row reaching the root through a repo-scoped folder
// runs, since that folder's parent is the repo header. nil leaves such a row resolving nothing.
type RepoRoots interface {
	DefaultWorkspaceOf(
		ctx context.Context,
		repoID string,
	) (string, error)
}

// cwdWalk answers where rowID's CLI runs: the nearest ancestor-or-self with a
// WorkspaceID, walking past an unprovisioned row. With none it reports the repo
// of the outermost repo-scoped folder crossed, for the RepoRoots fallback.
func cwdWalk(
	t tree.Tree,
	chats map[string]domain.Chat,
	rowID string,
) (workspaceID, repoID string, ok bool) {
	seen := map[string]bool{}
	for id := rowID; id != "" && !seen[id]; {
		seen[id] = true
		if c, ok := chats[id]; ok {
			if c.WorkspaceID != "" {
				return c.WorkspaceID, "", true
			}
			if c.Type == domain.ChatTypeFolder && c.RepoID != "" {
				repoID = c.RepoID
			}
		}
		node, ok := t.Node(id)
		if !ok {
			break
		}
		id = node.ParentID
	}
	return "", repoID, false
}

// repoRootFallback finishes a walk that found no workspace but crossed a
// repo-scoped folder: the answer is that repo's default checkout.
func repoRootFallback(
	ctx context.Context,
	roots RepoRoots,
	repoID string,
) (string, bool) {
	if roots == nil || repoID == "" {
		return "", false
	}
	id, err := roots.DefaultWorkspaceOf(ctx, repoID)
	if err != nil || id == "" {
		return "", false
	}
	return id, true
}

// errNoNodes is readNodeTable's answer when no Node port is wired.
var errNoNodes = errors.New("tree: no node port wired")

// nodeTable is one read of the Node table, indexed by id and by parent: each read decodes every row, and a
// walk asks about every level.
type nodeTable struct {
	byID     map[string]dtree.Node
	byParent map[string][]dtree.Node
}

// readNodeTable reads the table once; a nil port reads as an error so the walk
// skips its folder and anchor augmentation, as it does with no port wired.
func readNodeTable(
	ctx context.Context,
	nodes Nodes,
) (nodeTable, error) {
	if nodes == nil {
		return nodeTable{}, errNoNodes
	}
	all, err := nodes.ListAll(ctx)
	if err != nil {
		return nodeTable{}, err
	}
	t := nodeTable{
		byID:     make(map[string]dtree.Node, len(all)),
		byParent: make(map[string][]dtree.Node),
	}
	for _, n := range all {
		t.byID[n.ID] = n
		t.byParent[n.ParentID] = append(t.byParent[n.ParentID], n)
	}
	return t, nil
}

// CwdWorkspaceIDs answers the cwd walk for every row in one pass over one forest, omitting rows whose
// ancestry owns no workspace. Reachable folders and anchors are folded in first so a walk passes through
// them; either port may be nil, which skips that augmentation.
func CwdWorkspaceIDs(
	ctx context.Context,
	folders Folders,
	nodes Nodes,
	roots RepoRoots,
	rows []domain.Chat,
) map[string]string {
	forest, byID := buildForest(ctx, folders, nodes, rows)
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		id, repoID, ok := cwdWalk(forest, byID, row.ID)
		if !ok {
			id, ok = repoRootFallback(ctx, roots, repoID)
		}
		if ok {
			out[row.ID] = id
		}
	}
	return out
}

// buildForest folds every reachable folder and workspace-anchor row into rows
// and builds the tree plus id->row map every walk here needs.
func buildForest(
	ctx context.Context,
	folders Folders,
	nodes Nodes,
	rows []domain.Chat,
) (tree.Tree, map[string]domain.Chat) {
	if table, err := readNodeTable(ctx, nodes); err == nil {
		if folderRows, ferr := tree.FoldersBelow(ctx, folders, table.byParent, rows); ferr == nil {
			rows = append(rows, folderRows...)
		}
		rows = append(rows, workspaceAnchorsReachable(table, rows)...)
	}
	treeNodes := make([]tree.Node, len(rows))
	byID := make(map[string]domain.Chat, len(rows))
	for i, row := range rows {
		treeNodes[i] = tree.Node{ID: row.ID, ParentID: row.ParentID, Order: row.Order, CreatedAt: row.CreatedAt}
		byID[row.ID] = row
	}
	return tree.New(treeNodes), byID
}

// workspaceAnchorsReachable renders every workspace anchor Node some row hangs off, directly or through
// other anchors: a row placed under a chatless workspace would otherwise walk into an unknown id.
func workspaceAnchorsReachable(
	table nodeTable,
	rows []domain.Chat,
) []domain.Chat {
	known := make(map[string]bool, len(rows))
	for _, row := range rows {
		known[row.ID] = true
	}
	var found []domain.Chat
	for _, row := range rows {
		for id := row.ParentID; id != "" && !known[id]; {
			known[id] = true
			n, ok := table.byID[id]
			if !ok || n.Kind != dtree.KindWorkspace {
				break
			}
			found = append(found, workspaceAnchorView(id, n, time.Time{}))
			id = n.ParentID
		}
	}
	return found
}

// freshForest builds the forest a Resolve* walk needs from every row, with rowID's own row taken from the
// log fold: it may have been placed moments ago, so the projection can still be stale or missing it.
// Reachable folders and anchors are folded in; a nil Folders or Nodes port skips that.
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
	t, byID := buildForest(ctx, folders, nodes, corrected(rows, subject))
	return t, byID, nil
}

// ResolveForkParent is the cwd walk started one row up, over freshForest's read, for a caller with no
// snapshot of its own to walk (Promote, own_worktree.go).
func ResolveForkParent(
	ctx context.Context,
	chats Chats,
	folders Folders,
	nodes Nodes,
	roots RepoRoots,
	rowID string,
) (string, bool, error) {
	t, byID, err := freshForest(ctx, chats, folders, nodes, rowID)
	if err != nil {
		return "", false, err
	}
	node, ok := t.Node(rowID)
	if !ok {
		return "", false, nil
	}
	id, repoID, ok := cwdWalk(t, byID, node.ParentID)
	if !ok {
		id, ok = repoRootFallback(ctx, roots, repoID)
	}
	return id, ok, nil
}

// ResolveCwdWorkspaceID is cwdWalk over freshForest's log-corrected
// read — the spawn path's own read+build step, alongside ResolveForkParent's.
func ResolveCwdWorkspaceID(
	ctx context.Context,
	chats Chats,
	folders Folders,
	nodes Nodes,
	roots RepoRoots,
	rowID string,
) (string, bool, error) {
	t, byID, err := freshForest(ctx, chats, folders, nodes, rowID)
	if err != nil {
		return "", false, err
	}
	id, repoID, ok := cwdWalk(t, byID, rowID)
	if !ok {
		id, ok = repoRootFallback(ctx, roots, repoID)
	}
	return id, ok, nil
}
