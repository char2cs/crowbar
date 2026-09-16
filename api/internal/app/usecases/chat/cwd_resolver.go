package chat

import (
	"context"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
)

// cwdResolver adapts the chat repository to runner.AncestorCwd over
// tree.ResolveCwdWorkspaceID, so the runner component can resolve a bubble's
// cwd without importing internal/tree directly — the two are peers
// (aliases_test.go's layering rule), and this package is where they may both
// be named.
//
// folders/nodes (2026-09-08 sidebar-placement-unification Task 8's own
// review fix round) let the walk step through a Folder-only ancestor —
// see tree.ResolveCwdWorkspaceID's own doc.
type cwdResolver struct {
	chats   agentchat.EventStore
	folders TreeFolders
	nodes   TreeNodes
}

func (r cwdResolver) ResolveCwdWorkspaceID(
	ctx context.Context,
	chatID string,
) (string, bool, error) {
	return tree.ResolveCwdWorkspaceID(ctx, r.chats, r.folders, r.nodes, chatID)
}
