package tree

import (
	"slices"

	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree/internal/lineage"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// treeSnapshot is a set of Chat rows as of a single read, paired with the
// kind-agnostic plan that does the ordering. Every guard and every renumber
// runs against it, so an operation reads the world once and writes only what
// moved.
//
// Folder rows and conversation rows are the same aggregate now, so there is
// only ONE slice here, unlike the sidebar's own snapshot of this shape — a
// row's Type says what it is, and its ParentID is both where the panel draws
// it and, for a chat row, what it reads.
//
// Placement lives in the PLAN alone and is stamped back onto a domain row only
// when that row is about to be returned or written. Mirroring it into the rows
// as well would give every move two sources of truth to keep in step, and the
// one that lost would be whichever the next read happened to consult.
type treeSnapshot struct {
	rows []domain.Chat
	at   map[string]int
	plan tree.Tree
}

// newTreeSnapshot builds the snapshot over one read of Chat rows — either one
// workspace's (chat placement) or the wider set folder CRUD plans against (see
// Chats.ListChats).
func newTreeSnapshot(
	rows []domain.Chat,
) *treeSnapshot {
	t := &treeSnapshot{
		rows: rows,
		at:   make(map[string]int, len(rows)),
	}
	for i, row := range rows {
		t.at[row.ID] = i
	}
	t.plan = tree.New(t.nodes())
	return t
}

// nodes flattens the rows into the plan's sibling space. A chat carries its
// CreatedAt so a level nobody has dragged yet still comes back in a stable
// order: every such row holds Order 0, and without the tiebreak two identical
// reads can answer in two different sequences.
func (t *treeSnapshot) nodes() []tree.Node {
	nodes := make([]tree.Node, 0, len(t.rows))
	for _, row := range t.rows {
		nodes = append(nodes, tree.Node{
			ID:        row.ID,
			ParentID:  row.ParentID,
			Order:     row.Order,
			CreatedAt: row.CreatedAt,
		})
	}
	return nodes
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

// placedRow returns a row with the placement the plan settled on stamped onto
// it. Nothing else reads a stored ParentID or Order once a plan is running:
// they are the plan's answer, and a row that skipped this step would be saved
// with the placement it had before the drag.
func (t *treeSnapshot) placedRow(
	id string,
) *domain.Chat {
	row := t.row(id)
	node, ok := t.plan.Node(id)
	if row == nil || !ok {
		return nil
	}
	row.ParentID = node.ParentID
	row.Order = node.Order
	return row
}

func (t *treeSnapshot) add(
	row domain.Chat,
) {
	t.at[row.ID] = len(t.rows)
	t.rows = append(t.rows, row)
	t.plan.Add(tree.Node{ID: row.ID, ParentID: row.ParentID, Order: row.Order})
}

// drop removes a row from the snapshot and from the plan. The index map is
// rebuilt because the slice delete shifts every entry after the hole.
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

// subtree returns every row BELOW id, deepest first, split by kind.
//
// Deepest first is the order the cascade needs, not a detail of the walk: each
// chat is erased as a LEAF, so no half-finished cascade ever leaves a chat
// pointing at a parent that is already gone — which for this tree is not an
// orphan but a context read that resolves to nothing.
func (t *treeSnapshot) subtree(
	id string,
) (chats, folders []string) {
	for _, child := range t.plan.Members(id) {
		chats, folders = t.appendSubtree(child.ID, chats, folders)
	}
	return chats, folders
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
	return row != nil && row.Type == domain.ChatTypeChat
}

// chatLineage returns the CHAT ancestors of id as THE PLAN currently has them,
// nearest first, with folders stepped through.
//
// It reads the plan and never the rows, because a move has to be answered before
// it is written: the lineage a drag is about to create exists nowhere but here,
// and a store re-read would serve the tree exactly as it was before the drag —
// which for the one comparison this feeds is the same as not asking.
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
