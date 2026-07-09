package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// OpenSegment appends a new active segment (switch-in / resume).
type OpenSegment struct {
	ChatID           string
	SegmentID        string
	CrowbarSegmentID string
	ProviderID       string
	TerminalSession  string
	Now              time.Time
}

func (c OpenSegment) AggregateID() string  { return c.ChatID }
func (c OpenSegment) EventName() string    { return "agentchat.segment_opened." + c.ChatID }
func (c OpenSegment) ShouldSnapshot() bool { return false }

func (c OpenSegment) Validate(current *domain.AgentChat) error {
	if current == nil {
		return fmt.Errorf("open segment: no chat: %w", asynxModels.ErrValidation)
	}
	for _, s := range current.Segments {
		if s.Status == "active" {
			return fmt.Errorf("open segment: active segment exists: %w", asynxModels.ErrValidation)
		}
	}
	if c.SegmentID == "" {
		return fmt.Errorf("open segment: missing segment id: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c OpenSegment) EmitEvent(current *domain.AgentChat) domain.AgentChat {
	next := *current
	next.Segments = append(append([]domain.AgentSegment{}, current.Segments...), domain.AgentSegment{
		ID:                c.SegmentID,
		ProviderID:        c.ProviderID,
		CrowbarSegmentID:  c.CrowbarSegmentID,
		TerminalSessionID: c.TerminalSession,
		StartedAt:         c.Now,
		Status:            "active",
	})
	next.ActiveSegmentID = c.SegmentID
	next.LastActivityAt = c.Now
	return next
}
