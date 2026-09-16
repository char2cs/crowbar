package tree

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/cascade"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// subtreeIDsOf returns rootID and everything below it in rows, the set a move
// or delete takes as one — the same cascade.Plan the workspace hierarchy's own
// cascade delete already reduces its subtree to, fed the unified Chat rows
// instead of domain.Workspace rows. No row here carries a lock of its own yet,
// so every node is eligible.
func subtreeIDsOf(
	rootID string,
	rows []domain.Chat,
) []string {
	nodes := make([]cascade.Node, 0, len(rows))
	for _, row := range rows {
		nodes = append(nodes, cascade.Node{ID: row.ID, Parent: row.ParentID})
	}
	return cascade.Plan(rootID, nodes)
}
