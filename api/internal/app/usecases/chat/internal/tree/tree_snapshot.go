package tree

import (
	"slices"

	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree/internal/lineage"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/cascade"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// treeSnapshot is a set of Chat-shaped rows (chats, folders, repo phantoms, anchors) as of one read, with
// the kind-agnostic plan that orders them; every guard and renumber runs against it. Placement lives in the
// plan alone and is stamped onto a row only when it is returned or written, so there is one source of truth.
type treeSnapshot struct {
	rows []domain.Chat
	at   map[string]int
	plan tree.Tree
	// homeIDs marks every row whose placement is written through Nodes rather than Chats. Always non-nil.
	homeIDs map[string]bool
	// freshIDs is homeIDs' subset with no Node row yet, minted by Nodes.Create. Always non-nil.
	freshIDs map[string]bool
	// foreign marks root rows of another level sharing "" (another workspace's root chats), planned under
	// foreignRoot so a root densify never counts or writes them while walks still resolve through them.
	foreign map[string]bool
	// aliases folds the ids that name one level onto the id the plan keys
	// it by (see levelAliases). A row keeps the id it was filed under.
	aliases levelAliases
	// restated marks rows whose stored parent changed within one level (an
	// alias for the same container) — a placement write, not a reorder.
	restated map[string]bool
}

// foreignRoot is the plan-only container the out-of-scope root rows hang
// under; nothing is ever placed there and nothing is ever written back with it.
const foreignRoot = "\x00foreign-root"

// newTreeSnapshotScoped builds the snapshot over one read of Chat rows, with the
// rows foreign to its root level named (see treeSnapshot.foreign) and the level
// aliases the plan folds together.
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

// nodes flattens the rows into the plan's sibling space. CreatedAt breaks ties so a never-dragged level,
// all at Order 0, reads back in a stable order.
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

// rankOf is the tie rank of a Chat-shaped row — see tree.Node.Rank. A chat
// that owns its workspace (recorded, or a legacy branch row) IS that
// workspace's branch row on the sidebar.
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

// canonical is the plan's container id for id — see levelAliases.
func (t *treeSnapshot) canonical(
	id string,
) string {
	return t.aliases.canonical(id)
}

// reparented reports whether a row's stored parent must be written whole:
// the plan moved it between levels, or a placement restated it within one.
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

// placedRow returns a row with the plan's placement stamped onto it; a row saved without this step would
// keep the placement it had before the drag.
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

// subtree returns every row below id, deepest first, split by kind. The cascade erases each chat as a
// leaf, so a half-finished delete never leaves a chat reading a parent that is gone.
func (t *treeSnapshot) subtree(
	id string,
) (chats, folders []string) {
	for _, child := range t.plan.Members(t.container(id)) {
		chats, folders = t.appendSubtree(child.ID, chats, folders)
	}
	return chats, folders
}

// container is the plan container id's own children hang under: its level's
// canonical id, unless that is the root — a row whose level IS the root (the
// header's or the home's owner) holds nothing of its own.
func (t *treeSnapshot) container(
	id string,
) string {
	if c := t.canonical(id); c != "" {
		return c
	}
	return id
}

// subtreeIDs is cascade.Plan over the plan's containers, so rows filed under
// either id of one level are one subtree.
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

// chatLineage returns id's chat ancestors as the plan has them, nearest first, folders stepped through. It
// reads the plan, not the store, because the lineage a drag is about to create exists only here.
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
