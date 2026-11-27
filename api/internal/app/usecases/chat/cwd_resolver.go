package chat

import (
	"context"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
)

type cwdResolver struct {
	chats   agentchat.EventStore
	folders TreeFolders
	nodes   TreeNodes
	roots   TreeRepoRoots
}

func (r cwdResolver) ResolveCwdWorkspaceID(
	ctx context.Context,
	chatID string,
) (string, bool, error) {
	return tree.ResolveCwdWorkspaceID(ctx, r.chats, r.folders, r.nodes, r.roots, chatID)
}
