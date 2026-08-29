package tree

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree/internal/lineage"
)

// Lineage reads a chat's CHAT ancestors — what a thread inherits — out of the
// same two tables this usecase re-places rows in.
//
// It is the tree's, because the parent edge is the tree's: the walk that answers
// "what does this chat read" and the planner that decides where a drag may land
// are the same traversal over the same rows, and two implementations of it would
// disagree the first time folders changed meaning.
type Lineage = lineage.Resolver

// NewLineage builds the lineage reader over the chat repository — folder rows
// and chat rows are the same table now, so one read suffices where a second
// once resolved the chat-folder table.
//
// It is assembled BEFORE the tree usecase and independently of it, and that is
// deliberate: the tree usecase holds the chat usecase (a chat delete cascades
// through it) while the chat usecase needs lineage at spawn time, so hanging the
// read off the tree usecase would close a construction cycle around a store
// either of them can simply read.
func NewLineage(chats Chats) *Lineage {
	return lineage.New(chats)
}
