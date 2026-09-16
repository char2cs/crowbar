package worktree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Folders/Nodes are the narrow Folder+Node read ports newChatForest needs to
// see PAST a folder ancestor — 2026-09-08 sidebar-placement-unification
// Task 8's own review fix round. A folder is a domain.Folder+domain.Node
// pair now (never a Chat row, home-scoped since Task 5, repo-scoped too
// since Task 8), so the raw ChatLister read this package's own ancestry/
// workspace-fan-out walks were built over no longer carries it at all — a
// chat filed under a folder nested somewhere in a branch's own subtree
// (the ordinary, expected sidebar organization pattern) had its ancestry
// walk stop dead the moment it reached that folder, even when a real
// worktree-owning ancestor sits one hop further up.
//
// Declared here rather than imported from usecases/chat/internal/tree (law
// 3, law 4 — this package's own existing convention, see ChatLister's doc):
// this package cannot import usecases/chat's internals, so it declares its
// own narrow shape and lets the container-layer adapters (repos.Folder,
// repos.Node) satisfy it structurally.
type Folders interface {
	FindByKey(ctx context.Context, id string) (*domain.Folder, error)
}

type Nodes interface {
	ListByParent(ctx context.Context, parentID string) ([]domain.Node, error)
}

// chatForest is one read of the whole chat/folder placement tree, indexed for
// upward walks. Both directions of the resolver build it ONCE per call: an
// ancestry walk cannot know which rows sit above chatID without the rest of the
// forest, and ChatsForWorkspace would otherwise re-read it per chat.
type chatForest struct {
	byID map[string]domain.Chat
	tree tree.Tree
}

// newChatForest folds every Folder+Node row reachable from rows' own ids
// into rows FIRST (foldersReachableFromRoots — mirrors usecases/chat's own
// identically-shaped fix, home_read.go/walk.go, duplicated rather than
// imported for the same law-3/law-4 reason Folders/Nodes above are declared
// locally), so the walk below can step straight through a folder ancestor
// the same way it already does for an ordinary chat-typed container. folders
// or nodes may be nil (a caller with neither wired) — the walk then degrades
// to its pre-Task-8, Chat-only behaviour rather than failing.
func newChatForest(
	ctx context.Context,
	folders Folders,
	nodes Nodes,
	rows []domain.Chat,
) chatForest {
	if folderRows, err := foldersReachableFromRoots(ctx, folders, nodes, rows); err == nil {
		rows = append(rows, folderRows...)
	}
	byID := make(map[string]domain.Chat, len(rows))
	treeNodes := make([]tree.Node, len(rows))
	for i, row := range rows {
		byID[row.ID] = row
		treeNodes[i] = tree.Node{ID: row.ID, ParentID: row.ParentID}
	}
	return chatForest{byID: byID, tree: tree.New(treeNodes)}
}

// foldersReachableFromRoots discovers every Folder+Node row filed somewhere
// between baseRows' own ids and the panel root, rendered as the same
// Chat-shaped view PlaceChat itself uses. Mirrors usecases/chat's own
// foldersReachableFrom (home_read.go)/foldersReachableFromRoots (walk.go)
// exactly — same BFS shape: seed the queue with "" plus every id baseRows
// already names, so a folder nested under an ordinary chat or a locked
// branch's own un-Noded owning row is still discovered.
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

func (f chatForest) ancestry(
	chatID string,
) []domain.Chat {
	ancestry := make([]domain.Chat, 0, 4)
	seen := map[string]bool{}
	for id := chatID; id != "" && !seen[id]; id = f.parentOf(id) {
		seen[id] = true
		row, ok := f.byID[id]
		if !ok {
			return ancestry
		}
		ancestry = append(ancestry, row)
		if row.WorkspaceID != "" {
			return ancestry
		}
	}
	return ancestry
}

func (f chatForest) workspaceFor(
	chatID string,
	memo map[string]string,
) string {
	walked := make([]string, 0, 4)
	workspaceID := ""
	seen := map[string]bool{}
	for id := chatID; id != "" && !seen[id]; id = f.parentOf(id) {
		if known, ok := memo[id]; ok {
			workspaceID = known
			break
		}
		seen[id] = true
		row, ok := f.byID[id]
		if !ok {
			break
		}
		walked = append(walked, id)
		if row.WorkspaceID != "" {
			workspaceID = row.WorkspaceID
			break
		}
	}
	for _, id := range walked {
		memo[id] = workspaceID
	}
	return workspaceID
}

func (f chatForest) parentOf(
	id string,
) string {
	node, ok := f.tree.Node(id)
	if !ok {
		return ""
	}
	return node.ParentID
}
