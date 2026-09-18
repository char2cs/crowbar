package commands

import (
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	asynxModels "github.com/char2cs/asynx/models"
)

// BindSession records the provider's conversation id for a runner that is
// staying put. It is the reducer's "bound" outcome: the runner announced its
// FIRST conversation. A runner that announces a DIFFERENT conversation is
// Move-ing, not binding.
type BindSession struct {
	RunnerID  string
	SessionID string
	Resumable bool
	Now       time.Time
	// Model and Effort are what the provider's OWN session-start payload
	// reported, when its descriptor maps one (e.g. claude.yaml's session_start
	// -> model) — the one moment some CLIs ever say which concrete model they
	// resolved to, since none expose it queryable afterwards. Empty means the
	// provider's hook carries none; the aggregate's existing LaunchModel/
	// LaunchEffort (set at spawn, possibly "" for "provider's own default")
	// are left untouched rather than being clobbered blank.
	Model  string
	Effort string
}

func (c BindSession) AggregateID() string  { return c.RunnerID }
func (c BindSession) EventName() string    { return "runner.session_bound." + c.RunnerID }
func (c BindSession) ShouldSnapshot() bool { return false }

func (c BindSession) Validate(current *agents.Runner) error {
	if current == nil {
		return fmt.Errorf("bind session: no runner: %w", asynxModels.ErrValidation)
	}
	if c.SessionID == "" {
		return fmt.Errorf("bind session: missing session id: %w", asynxModels.ErrValidation)
	}
	// A conversation with no opening time would stamp FirstSeenAt zero and drop
	// history ordering back onto insertion order.
	if c.Now.IsZero() {
		return fmt.Errorf("bind session: missing timestamp: %w", asynxModels.ErrValidation)
	}
	return nil
}

// EmitEvent binds the conversation AND stamps when it opened. Now is carried
// onto the aggregate (never dropped): the conversation projection reads it for
// FirstSeenAt, and the runner's own StartedAt cannot stand in for it — a runner
// binds its conversation after it spawns, sometimes hours after.
func (c BindSession) EmitEvent(current *agents.Runner) agents.Runner {
	next := *current
	next.CurrentSession = c.SessionID
	next.CurrentSessionSince = c.Now
	next.CurrentSessionResumable = c.Resumable
	if c.Model != "" {
		next.LaunchModel = c.Model
	}
	if c.Effort != "" {
		next.LaunchEffort = c.Effort
	}
	return next
}
