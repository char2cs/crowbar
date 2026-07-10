package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// EndSegment ends the active segment (switch-out / process exit) — but ONLY
// when SegmentID still names the chat's active segment at apply time. That
// identity guard closes a TOCTOU: asynx dispatches commands async (ax.Send with
// dispatch lag), so a caller that decided "end the old segment" from an earlier
// read can have its EndSegment land AFTER a concurrent SwitchProvider opened a
// NEW segment. Comparing against the fresh event-log state inside EmitEvent
// makes the check-and-act atomic, so a stale EndSegment can never end (and
// silently orphan) the brand-new segment — it is a no-op instead.
type EndSegment struct {
	ChatID    string
	SegmentID string
	Now       time.Time
}

func (c EndSegment) AggregateID() string  { return c.ChatID }
func (c EndSegment) EventName() string    { return "agentchat.segment_ended." + c.ChatID }
func (c EndSegment) ShouldSnapshot() bool { return false }

func (c EndSegment) Validate(current *domain.AgentChat) error {
	if current == nil {
		return fmt.Errorf("end segment: no chat: %w", asynxModels.ErrValidation)
	}
	// Lenient by design: a no-match (already ended, or the segment the caller
	// meant is no longer active) is an idempotent no-op fold, never an error.
	return nil
}

func (c EndSegment) EmitEvent(current *domain.AgentChat) domain.AgentChat {
	// Identity guard: only end the segment the caller meant, and only while it
	// is still THE active one. If a concurrent switch has since made a different
	// segment active, this is a no-op — the new segment (and its live CLI) is
	// left untouched. Returning *current shares the (unmutated) Segments backing
	// array, which is safe because this path never writes to it.
	if current.ActiveSegmentID != c.SegmentID {
		return *current
	}
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
