package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// AgentWorkspaceReaper fakes the chat tree's WorkspaceReaper seam. Reaped records torn-down ids in order
// (deepest first is the contract); Err/ErrFor fail every/one reap; Roster is the census reaps remove from;
// Risks answers WorkAtRisk, Consents records each delete's, Takes names the lineage descendants taken.
type AgentWorkspaceReaper struct {
	Takes    map[string][]string
	Reaped   []string
	Roster   *AgentWorkspaceRoster
	Err      error
	ErrFor   map[string]error
	Risks    map[string]domain.WorkAtRisk
	Consents []domain.DeleteConsent
}

// NewAgentWorkspaceReaper returns a reaper that tears everything down happily.
func NewAgentWorkspaceReaper() *AgentWorkspaceReaper {
	return &AgentWorkspaceReaper{ErrFor: map[string]error{}}
}

// NewAgentWorkspaceReaperOver returns a reaper that removes what it reaps from
// roster, so "is it gone?" is a question about the census rather than about the
// call log.
func NewAgentWorkspaceReaperOver(
	roster *AgentWorkspaceRoster,
) *AgentWorkspaceReaper {
	return &AgentWorkspaceReaper{ErrFor: map[string]error{}, Roster: roster}
}

func (s *AgentWorkspaceReaper) DeleteWorkspace(
	ctx context.Context,
	workspaceID string,
	consent domain.DeleteConsent,
) error {
	s.Consents = append(s.Consents, consent)
	taken, err := s.DeletionPlan(ctx, workspaceID)
	if err != nil {
		return err
	}
	s.Reaped = append(s.Reaped, workspaceID)
	if s.Roster != nil {
		gone := map[string]bool{}
		for _, id := range taken {
			gone[id] = true
		}
		kept := make([]domain.Workspace, 0, len(s.Roster.Rows))
		for _, w := range s.Roster.Rows {
			if !gone[w.ID] {
				kept = append(kept, w)
			}
		}
		s.Roster.Rows = kept
	}
	return nil
}

// DeletionPlan answers the refusal DeleteWorkspace would, else what it takes.
func (s *AgentWorkspaceReaper) DeletionPlan(
	_ context.Context,
	workspaceID string,
) ([]string, error) {
	if err, ok := s.ErrFor[workspaceID]; ok {
		return nil, err
	}
	if s.Err != nil {
		return nil, s.Err
	}
	return append(append([]string{}, s.Takes[workspaceID]...), workspaceID), nil
}

func (s *AgentWorkspaceReaper) WorkAtRisk(
	_ context.Context,
	workspaceIDs []string,
) ([]domain.WorkAtRisk, error) {
	var risks []domain.WorkAtRisk
	for _, id := range workspaceIDs {
		if risk, ok := s.Risks[id]; ok {
			risks = append(risks, risk)
		}
	}
	return risks, nil
}
