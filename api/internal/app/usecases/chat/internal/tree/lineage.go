package tree

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree/internal/lineage"
)

// Lineage reads a chat's chat ancestors, what a thread inherits. It lives with the tree because the parent
// edge is the tree's: lineage and drag planning are one traversal and must not disagree.
type Lineage = lineage.Resolver

// NewLineage builds the lineage reader over the chat repository, independently of the tree usecase: that
// usecase holds the chat usecase, which needs lineage at spawn time, so the reverse would be a cycle.
func NewLineage(chats Chats) *Lineage {
	return lineage.New(chats)
}
