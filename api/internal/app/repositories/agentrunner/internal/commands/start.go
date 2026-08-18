package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Start records a freshly-spawned CLI. No conversation is bound yet — the
// provider announces one via its session hook, which lands as BindSession.
type Start struct {
	RunnerID        string
	WorkspaceID     string
	ProviderID      string
	TerminalSession string
	ChatID          string
	LaunchSessionID string
	LaunchModel     string
	LaunchEffort    string
	Now             time.Time
}

func (c Start) AggregateID() string  { return c.RunnerID }
func (c Start) EventName() string    { return "agentrunner.started." + c.RunnerID }
func (c Start) ShouldSnapshot() bool { return false }

func (c Start) Validate(current *domain.AgentRunner) error {
	if current != nil {
		return fmt.Errorf("start runner: already started: %w", asynxModels.ErrValidation)
	}
	if c.RunnerID == "" {
		return fmt.Errorf("start runner: missing runner id: %w", asynxModels.ErrValidation)
	}
	// Invariant I1: a runner points at exactly one chat, always.
	if c.ChatID == "" {
		return fmt.Errorf("start runner: missing chat id: %w", asynxModels.ErrValidation)
	}
	if c.TerminalSession == "" {
		return fmt.Errorf("start runner: missing terminal session: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Start) EmitEvent(_ *domain.AgentRunner) domain.AgentRunner {
	return domain.AgentRunner{
		ID:              c.RunnerID,
		WorkspaceID:     c.WorkspaceID,
		ProviderID:      c.ProviderID,
		TerminalSession: c.TerminalSession,
		CurrentChatID:   c.ChatID,
		LaunchSessionID: c.LaunchSessionID,
		LaunchModel:     c.LaunchModel,
		LaunchEffort:    c.LaunchEffort,
		StartedAt:       c.Now,
	}
}
