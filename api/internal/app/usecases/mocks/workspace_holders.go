package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
)

// AgentWorkspaceHolders fakes the chat tree's WorkspaceHolders seam over AgentChatPlacements, deriving the
// answer with the real worktree.ChatsForWorkspace so inherited worktrees are counted as in production.
// Err fails the census, to prove a delete refuses when it cannot tell whether a worktree is shared.
type AgentWorkspaceHolders struct {
	Chats *AgentChatPlacements
	Err   error
}

// NewAgentWorkspaceHolders returns a census over chats' own rows.
func NewAgentWorkspaceHolders(
	chats *AgentChatPlacements,
) *AgentWorkspaceHolders {
	return &AgentWorkspaceHolders{Chats: chats}
}

func (s *AgentWorkspaceHolders) ChatsForWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]string, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	// nil, nil: this fake's fixtures are chat-only and need no Folder/Node awareness.
	return worktree.ChatsForWorkspace(ctx, workspaceID, s.Chats, nil, nil)
}
