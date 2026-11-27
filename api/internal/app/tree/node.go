package tree

import (
	"strings"
	"time"
)

type Node struct {
	ID        string
	ParentID  string
	Order     int
	CreatedAt time.Time
	Rank      int
}

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
