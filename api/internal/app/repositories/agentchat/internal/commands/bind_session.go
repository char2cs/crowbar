package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// BindSession records the provider's session id on the segment matching a
// CrowbarSegmentID (once the provider reports it for the live turn).
type BindSession struct {
	ChatID            string
	CrowbarSegmentID  string
	ProviderSessionID string
}

func (c BindSession) AggregateID() string  { return c.ChatID }
func (c BindSession) EventName() string    { return "agentchat.session_bound." + c.ChatID }
func (c BindSession) ShouldSnapshot() bool { return false }

func (c BindSession) Validate(current *domain.AgentChat) error {
	if current == nil {
		return fmt.Errorf("bind session: no chat: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c BindSession) EmitEvent(current *domain.AgentChat) domain.AgentChat {
	next := *current
	segs := append([]domain.AgentSegment{}, current.Segments...)
	for i := range segs {
		if segs[i].CrowbarSegmentID == c.CrowbarSegmentID && segs[i].Status == "active" {
			segs[i].ProviderSessionID = c.ProviderSessionID
		}
	}
	next.Segments = segs
	return next
}
