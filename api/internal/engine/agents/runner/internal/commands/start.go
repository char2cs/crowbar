package commands

import (
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	asynxModels "github.com/char2cs/asynx/models"
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
func (c Start) EventName() string    { return "runner.started." + c.RunnerID }
func (c Start) ShouldSnapshot() bool { return false }

func (c Start) Validate(current *agents.Runner) error {
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

func (c Start) EmitEvent(_ *agents.Runner) agents.Runner {
	return agents.Runner{
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
