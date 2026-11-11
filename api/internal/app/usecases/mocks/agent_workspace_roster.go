package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// AgentWorkspaceRoster fakes the WorkspaceRoster seam, answering in seeded order: unsorted
// on purpose, since the backfill's root-first ordering is under test.
type AgentWorkspaceRoster struct {
	Rows []domain.Workspace
	Err  error
}

// NewAgentWorkspaceRoster returns a roster holding no workspaces.
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
