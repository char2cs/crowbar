package tree

import (
	"slices"

	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree/internal/lineage"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/cascade"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type treeSnapshot struct {
	rows     []domain.Chat
	at       map[string]int
	plan     tree.Tree
	homeIDs  map[string]bool
	freshIDs map[string]bool
	foreign  map[string]bool
	aliases  levelAliases
	restated map[string]bool
}

const foreignRoot = "\x00foreign-root"

func newTreeSnapshotScoped(
	rows []domain.Chat,
	foreign map[string]bool,
	aliases levelAliases,
) *treeSnapshot {
	t := &treeSnapshot{
		rows:     rows,
		at:       make(map[string]int, len(rows)),
		homeIDs:  map[string]bool{},
		freshIDs: map[string]bool{},
		foreign:  foreign,
		aliases:  aliases,
		restated: map[string]bool{},
	}
	for i, row := range rows {
		t.at[row.ID] = i
	}
	t.plan = tree.New(t.nodes())
	return t
}

func (t *treeSnapshot) nodes() []tree.Node {
	nodes := make([]tree.Node, 0, len(t.rows))
	for _, row := range t.rows {
		parentID := t.canonical(row.ParentID)
		if t.foreign[row.ID] {
			parentID = foreignRoot
		}
		nodes = append(nodes, tree.Node{
			ID:        row.ID,
			ParentID:  parentID,
			Order:     row.Order,
			CreatedAt: row.CreatedAt,
			Rank:      rankOf(row),
		})
	}
	return nodes
}

func rankOf(
	row domain.Chat,
) int {
	switch row.Type {
	case domain.ChatTypeFolder:
		return tree.RankFolder
	case nodePhantomType:
		return tree.RankRepo
	case workspaceAnchorType, domain.ChatTypeBranch:
		return tree.RankWorkspace
	}
	if row.OwnsWorkspace {
		return tree.RankWorkspace
	}
	return tree.RankChat
}

func (t *treeSnapshot) canonical(
	id string,
) string {
	return t.aliases.canonical(id)
}

func (t *treeSnapshot) reparented(
	id string,
) bool {
	return t.plan.Reparented(id) || t.restated[id]
}

func (t *treeSnapshot) row(
	id string,
) *domain.Chat {
	at, ok := t.at[id]
	if !ok {
		return nil
	}
	return &t.rows[at]
}

func (t *treeSnapshot) placedRow(
	id string,
) *domain.Chat {
	row := t.row(id)
	node, ok := t.plan.Node(id)
	if row == nil || !ok {
		return nil
	}
	if t.canonical(row.ParentID) != node.ParentID {
		row.ParentID = node.ParentID
	}
	row.Order = node.Order
	return row
}

func (t *treeSnapshot) add(
	row domain.Chat,
) {
	t.at[row.ID] = len(t.rows)
	t.rows = append(t.rows, row)
	t.plan.Add(tree.Node{ID: row.ID, ParentID: t.canonical(row.ParentID), Order: row.Order, Rank: rankOf(row)})
}

func (t *treeSnapshot) drop(
	id string,
) {
	if _, ok := t.at[id]; !ok {
		return
	}
	t.plan.Drop(id)
	t.rows = slices.DeleteFunc(t.rows, func(row domain.Chat) bool { return row.ID == id })
	t.at = make(map[string]int, len(t.rows))
	for i, row := range t.rows {
		t.at[row.ID] = i
	}
}

func (t *treeSnapshot) subtree(
	id string,
) (chats, folders []string) {
	for _, child := range t.plan.Members(t.container(id)) {
		chats, folders = t.appendSubtree(child.ID, chats, folders)
	}
	return chats, folders
}

func (t *treeSnapshot) container(
	id string,
) string {
	if c := t.canonical(id); c != "" {
		return c
	}
	return id
}

func (t *treeSnapshot) subtreeIDs(
	id string,
) []string {
	nodes := make([]cascade.Node, 0, len(t.rows))
	for _, row := range t.rows {
		nodes = append(nodes, cascade.Node{ID: row.ID, Parent: t.container(row.ParentID)})
	}
	return cascade.Plan(id, nodes)
}

func (t *treeSnapshot) appendSubtree(
	id string,
	chats []string,
	folders []string,
) ([]string, []string) {
	deepChats, deepFolders := t.subtree(id)
	chats = append(chats, deepChats...)
	folders = append(folders, deepFolders...)
	if t.isFolder(id) {
		return chats, append(folders, id)
	}
	return append(chats, id), folders
}

func (t *treeSnapshot) isFolder(
	id string,
) bool {
	row := t.row(id)
	return row != nil && row.Type == domain.ChatTypeFolder
}

func (t *treeSnapshot) isChat(
	id string,
) bool {
	row := t.row(id)
	return row != nil && row.IsChat()
}

func (t *treeSnapshot) chatLineage(
	id string,
) []string {
	return lineage.Walk(id, t.planParentOf, t.isChat)
}

func (t *treeSnapshot) planParentOf(
	id string,
) string {
	node, _ := t.plan.Node(id)
	return node.ParentID
}
