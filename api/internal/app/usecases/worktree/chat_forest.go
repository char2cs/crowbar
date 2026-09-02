package worktree

import (
	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// chatForest is one read of the whole chat/folder placement tree, indexed for
// upward walks. Both directions of the resolver build it ONCE per call: an
// ancestry walk cannot know which rows sit above chatID without the rest of the
// forest, and ChatsForWorkspace would otherwise re-read it per chat.
type chatForest struct {
	byID map[string]domain.Chat
	tree tree.Tree
}

func newChatForest(
	rows []domain.Chat,
) chatForest {
	byID := make(map[string]domain.Chat, len(rows))
	nodes := make([]tree.Node, len(rows))
	for i, row := range rows {
		byID[row.ID] = row
		nodes[i] = tree.Node{ID: row.ID, ParentID: row.ParentID}
	}
	return chatForest{byID: byID, tree: tree.New(nodes)}
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
