package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// EndSegment ends the currently-active segment (switch-out / process exit).
type EndSegment struct {
	ChatID string
	Now    time.Time
}

func (c EndSegment) AggregateID() string  { return c.ChatID }
func (c EndSegment) EventName() string    { return "agentchat.segment_ended." + c.ChatID }
func (c EndSegment) ShouldSnapshot() bool { return false }

func (c EndSegment) Validate(current *domain.AgentChat) error {
	if current == nil {
		return fmt.Errorf("end segment: no chat: %w", asynxModels.ErrValidation)
	}
	return nil // idempotent: ending with no active segment is a no-op fold
}

func (c EndSegment) EmitEvent(current *domain.AgentChat) domain.AgentChat {
	next := *current
	segs := append([]domain.AgentSegment{}, current.Segments...)
	for i := range segs {
		if segs[i].ID == current.ActiveSegmentID && segs[i].Status == "active" {
			ended := c.Now
			segs[i].Status = "ended"
			segs[i].EndedAt = &ended
		}
	}
	next.Segments = segs
	next.ActiveSegmentID = ""
	next.LastActivityAt = c.Now
	return next
}
