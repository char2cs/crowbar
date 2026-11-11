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

// foldersReachableFromRoots is every folder below the root or rows; a nil port finds nothing.
func foldersReachableFromRoots(
	ctx context.Context, folders Folders, nodes Nodes, rows []domain.Chat,
) ([]domain.Chat, error) {
	if nodes == nil {
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
	return tree.FoldersBelow(ctx, folders, byParent, rows)
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
