package tree

import (
	"strings"
	"time"
)

// Node is one row in a sibling space: an id, the container it hangs off, its
// dense index inside that container, and the moment it was created.
//
// ParentID is whatever the caller has already resolved the row's container down
// to — another node's id, or "" for the root. The tree never looks behind it, so
// a caller whose rows carry several edges decides for itself which one is the
// container and keeps the rest to itself.
//
// CreatedAt is the tiebreak that keeps a never-ordered level still. Every row a
// user has not dragged carries Order 0, so without it two identical requests can
// return the same level in two different sequences and the sidebar reshuffles
// under the cursor. A caller with no creation time may leave it zero; such rows
// then tie among themselves and fall through to the id, which is stable too.
type Node struct {
	ID        string
	ParentID  string
	Order     int
	CreatedAt time.Time
	// Rank breaks an Order tie by row KIND before CreatedAt, in the sequence
	// the sidebar draws a level nobody has dragged (workspace-tree-utils.ts
	// buildSidebarTree: folders, branches, chats; a project's top level then
	// appends repo headers after its home rows). A densify must count a tied
	// level in that same sequence or the first drop lands slots off.
	Rank int
}

// Row kinds in the order a tied level is drawn. A workspace-OWNING chat is
// drawn as its branch row, so it ranks as a workspace, not a chat.
const (
	RankFolder = iota
	RankWorkspace
	RankChat
	RankRepo
)

func compareNodes(
	a Node,
	b Node,
) int {
	if a.Order != b.Order {
		return a.Order - b.Order
	}
	if a.Rank != b.Rank {
		return a.Rank - b.Rank
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Compare(b.CreatedAt)
	}
	return strings.Compare(a.ID, b.ID)
}
