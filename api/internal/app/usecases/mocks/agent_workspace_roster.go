package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type AgentWorkspaceRoster struct {
	Rows []domain.Workspace
	Err  error
}

func NewAgentWorkspaceRoster() *AgentWorkspaceRoster {
	return &AgentWorkspaceRoster{}
}

func (s *AgentWorkspaceRoster) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Rows, nil
}
