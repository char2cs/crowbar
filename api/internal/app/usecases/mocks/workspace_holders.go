package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
)

// AgentWorkspaceHolders fakes the chat tree usecase's WorkspaceHolders seam:
// every chat currently resolving to a workspace, the census a cascading delete
// checks before it may tear that worktree down.
//
// It is a fake only in WHERE the rows come from — the same AgentChatPlacements
// the tree itself plans over — and not in how the answer is derived: it calls
// the REAL worktree.ChatsForWorkspace, exactly as the container's own resolver
// does. A hand-rolled "chats whose WorkspaceID equals this" would have re-stated
// the very assumption the gate exists to correct, and would have passed while a
// bubble inheriting its ancestor's worktree went uncounted.
//
// Err fails the census, which is what a test needs to prove the failure
// contract: a delete that cannot establish whether a worktree is shared must
// refuse rather than assume it is not.
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
	return worktree.ChatsForWorkspace(ctx, workspaceID, s.Chats)
}
