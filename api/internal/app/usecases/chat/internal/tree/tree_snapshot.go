package tree

import (
	"slices"

	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree/internal/lineage"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// treeSnapshot is one workspace's Chats-panel rows as of a single read, paired
// with the kind-agnostic plan that does the ordering. Every guard and every
// renumber runs against it, so an operation reads the world once and writes only
// what moved.
//
// Placement lives in the PLAN alone and is stamped back onto a domain row only
// when that row is about to be returned or written. Mirroring it into the rows
// as well would give every move two sources of truth to keep in step, and the
// one that lost would be whichever the next read happened to consult.
//
// Unlike the sidebar's snapshot of the same shape, this one hides nothing from
// the plan: a chat has ONE parent edge, and that edge is both where the row is
// drawn and what it reads. So the container a chat hands over is simply its
// ParentID, and a drag that changes it legitimately changes lineage.
type treeSnapshot struct {
	folders  []domain.ChatFolder
	chats    []domain.Chat
	folderAt map[string]int
	chatAt   map[string]int
	plan     tree.Tree
}

// newTreeSnapshot builds the snapshot over one workspace's folders and chats.
// Both kinds go into a single sibling space because that is how the panel draws
// them: they interleave at every level and sort on one Order field.
func newTreeSnapshot(
	folders []domain.ChatFolder,
	chats []domain.Chat,
) *treeSnapshot {
	t := &treeSnapshot{
		folders:  folders,
		chats:    chats,
		folderAt: make(map[string]int, len(folders)),
		chatAt:   make(map[string]int, len(chats)),
	}
	for i, f := range folders {
		t.folderAt[f.ID] = i
	}
	for i, c := range chats {
		t.chatAt[c.ID] = i
	}
	t.plan = tree.New(t.nodes())
	return t
}

// nodes flattens both row kinds into the one sibling space they share. A chat
// carries its CreatedAt so a level nobody has dragged yet still comes back in a
// stable order: every such row holds Order 0, and without the tiebreak two
// identical reads can answer in two different sequences.
func (t *treeSnapshot) nodes() []tree.Node {
	nodes := make([]tree.Node, 0, len(t.folders)+len(t.chats))
	for _, f := range t.folders {
		nodes = append(nodes, tree.Node{ID: f.ID, ParentID: f.ParentID, Order: f.Order})
	}
	for _, c := range t.chats {
		nodes = append(nodes, tree.Node{
			ID:        c.ID,
			ParentID:  c.ParentID,
			Order:     c.Order,
			CreatedAt: c.CreatedAt,
		})
	}
	return nodes
}

func (t *treeSnapshot) folder(
	id string,
) *domain.ChatFolder {
	at, ok := t.folderAt[id]
	if !ok {
		return nil
	}
	return &t.folders[at]
}

func (t *treeSnapshot) chat(
	id string,
) *domain.Chat {
	at, ok := t.chatAt[id]
	if !ok {
		return nil
	}
	return &t.chats[at]
}

// placedFolder returns a folder row with the placement the plan settled on
// stamped onto it. Nothing else reads a stored ParentID or Order once a plan is
// running: they are the plan's answer, and a row that skipped this step would be
// saved with the placement it had before the drag.
func (t *treeSnapshot) placedFolder(
	id string,
) *domain.ChatFolder {
	row := t.folder(id)
	node, ok := t.plan.Node(id)
	if row == nil || !ok {
		return nil
	}
	row.ParentID = node.ParentID
	row.Order = node.Order
	return row
}

// placedChat returns a chat row with the plan's placement stamped on. Both
// fields, unlike the sidebar's workspace equivalent which may only take the
// index: a chat's parent is not a fact about anything on disk, so the plan owns
// it outright.
func (t *treeSnapshot) placedChat(
	id string,
) *domain.Chat {
	row := t.chat(id)
	node, ok := t.plan.Node(id)
	if row == nil || !ok {
		return nil
	}
	row.ParentID = node.ParentID
	row.Order = node.Order
	return row
}

func (t *treeSnapshot) add(
	row domain.ChatFolder,
) {
	t.folderAt[row.ID] = len(t.folders)
	t.folders = append(t.folders, row)
	t.plan.Add(tree.Node{ID: row.ID, ParentID: row.ParentID, Order: row.Order})
}

// drop removes a folder from the snapshot and from the plan. The index map is
// rebuilt because the slice delete shifts every entry after the hole.
func (t *treeSnapshot) drop(
	id string,
) {
	if _, ok := t.folderAt[id]; !ok {
		return
	}
	t.plan.Drop(id)
	t.folders = slices.DeleteFunc(t.folders, func(f domain.ChatFolder) bool { return f.ID == id })
	t.folderAt = make(map[string]int, len(t.folders))
	for i, f := range t.folders {
		t.folderAt[f.ID] = i
	}
}

// dropChat removes a chat from the snapshot and from the plan, for the cascade
// that takes a deleted chat's whole subtree with it. The plan must lose the row
// too, or the densify that follows would leave a hole where a chat that no
// longer exists used to sit.
func (t *treeSnapshot) dropChat(
	id string,
) {
	if _, ok := t.chatAt[id]; !ok {
		return
	}
	t.plan.Drop(id)
	t.chats = slices.DeleteFunc(t.chats, func(c domain.Chat) bool { return c.ID == id })
	t.chatAt = make(map[string]int, len(t.chats))
	for i, c := range t.chats {
		t.chatAt[c.ID] = i
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
	if t.folder(id) != nil {
		return chats, append(folders, id)
	}
	return append(chats, id), folders
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

func (t *treeSnapshot) isChat(
	id string,
) bool {
	return t.chat(id) != nil
}
