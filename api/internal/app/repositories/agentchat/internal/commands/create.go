package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Create seeds a new AgentChat with its first (active) segment.
type Create struct {
	ID               string
	WorkspaceID      string
	SegmentID        string
	CrowbarSegmentID string
	ProviderID       string
	TerminalSession  string
	Now              time.Time
}

func (c Create) AggregateID() string  { return c.ID }
func (c Create) EventName() string    { return "agentchat.created." + c.ID }
func (c Create) ShouldSnapshot() bool { return false }

func (c Create) Validate(current *domain.AgentChat) error {
	if current != nil {
		return fmt.Errorf("create agent chat: exists: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WorkspaceID == "" || c.SegmentID == "" {
		return fmt.Errorf("create agent chat: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Create) EmitEvent(_ *domain.AgentChat) domain.AgentChat {
	return domain.AgentChat{
		ID:              c.ID,
		WorkspaceID:     c.WorkspaceID,
		ActiveSegmentID: c.SegmentID,
		Segments: []domain.AgentSegment{{
			ID:                c.SegmentID,
			ProviderID:        c.ProviderID,
			CrowbarSegmentID:  c.CrowbarSegmentID,
			TerminalSessionID: c.TerminalSession,
			StartedAt:         c.Now,
			Status:            "active",
		}},
		CreatedAt:      c.Now,
		LastActivityAt: c.Now,
	}
}
