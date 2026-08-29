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
type cwdResolver struct {
	chats agentchat.EventStore
}

func (r cwdResolver) ResolveCwdWorkspaceID(
	ctx context.Context,
	chatID string,
) (string, bool, error) {
	return tree.ResolveCwdWorkspaceID(ctx, r.chats, chatID)
}
