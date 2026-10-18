package store

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// runnerRow is the LIVE-runner read model: one row per running CLI, DELETED on
// exit, so "does a row exist for this chat" IS the liveness question.
type runnerRow struct {
	ID                    string `gorm:"primaryKey"`
	WorkspaceID           string `gorm:"index"`
	ProviderID            string
	TerminalSession       string
	CurrentChatID         string `gorm:"index"`
	LaunchModel           string
	LaunchEffort          string
	ReportedModel         string
	ReportedEffort        string
	LaunchPermissionLevel string
	StartedAt             time.Time
}

func (runnerRow) TableName() string {
	return "agent_runners"
}

// toRunner maps a live row back to the aggregate's shape. ExitedAt is always nil
// by construction: an exited runner has no row (that is the whole point), so a
// row can only ever describe a runner the model believes is still running.
func (r runnerRow) toRunner() agents.Runner {
	return agents.Runner{
		ID:                    r.ID,
		WorkspaceID:           r.WorkspaceID,
		ProviderID:            r.ProviderID,
		TerminalSession:       r.TerminalSession,
		CurrentChatID:         r.CurrentChatID,
		LaunchModel:           r.LaunchModel,
		LaunchEffort:          r.LaunchEffort,
		ReportedModel:         r.ReportedModel,
		ReportedEffort:        r.ReportedEffort,
		LaunchPermissionLevel: r.LaunchPermissionLevel,
		StartedAt:             r.StartedAt,
	}
}
