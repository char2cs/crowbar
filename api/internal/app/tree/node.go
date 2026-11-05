package tree

import (
	"strings"
	"time"
)

// Node is one row in a sibling space. ParentID is the container the caller resolved,
// "" for root; the tree never looks behind it. CreatedAt breaks ties among undragged
// (Order 0) rows so a level never reshuffles; zero falls through to the stable id.
type Node struct {
	ID        string
	ParentID  string
	Order     int
	CreatedAt time.Time
	// Rank breaks an Order tie by row kind before CreatedAt, in the order the sidebar
	// draws an undragged level (folders, branches, chats, then repo headers); a densify
	// must count a tied level the same way or the first drop lands off.
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
