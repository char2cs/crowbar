package worktree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

// Folders/Nodes are the narrow Folder+Node read ports newChatForest needs to walk past a folder
// ancestor, which is no Chat row, to the worktree-owning ancestor above it. Declared locally because
// this package cannot import usecases/chat's internals; container adapters satisfy them structurally.
type Folders interface {
	FindByKey(ctx context.Context, id string) (*dtree.Folder, error)
}

type Nodes interface {
	ListAll(ctx context.Context) ([]dtree.Node, error)
}

// chatForest is one read of the whole chat/folder placement tree, indexed for upward walks; each
// resolver entry point builds it once rather than re-reading per chat.
type chatForest struct {
	byID map[string]domain.Chat
	tree tree.Tree
}

// newChatForest folds every Folder+Node row reachable from rows' ids into rows first, so the walk steps
// through folder ancestors like chat containers. Nil folders or nodes degrade it to a Chat-only walk.
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

// foldersReachableFromRoots BFS-discovers every Folder+Node row between baseRows' ids and the panel root,
// as Chat-shaped views, seeding "" plus every baseRows id so folders under chats are found too.
func foldersReachableFromRoots(
	ctx context.Context,
	folders Folders,
	nodes Nodes,
	baseRows []domain.Chat,
) ([]domain.Chat, error) {
	if folders == nil || nodes == nil {
		return nil, nil
	}
	all, err := nodes.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	byParent := make(map[string][]dtree.Node)
	for _, n := range all {
		byParent[n.ParentID] = append(byParent[n.ParentID], n)
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
		for _, n := range byParent[parent] {
			if n.Kind != dtree.KindFolder || seenFolder[n.ID] {
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
