package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
)

type AgentWorkspaceHolders struct {
	Chats *AgentChatPlacements
	Err   error
}

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
	return worktree.ChatsForWorkspace(ctx, workspaceID, s.Chats, nil, nil)
}
