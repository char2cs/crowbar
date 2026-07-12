package store

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// runnerRow is the LIVE-runner read model: one row per running CLI. A runner's
// row is DELETED on exit, so "does a row exist for this chat" IS the liveness
// question — there is no status column to go stale (spec §2). Liveness belongs
// to the PTY alone; this projection only records WHERE a still-running CLI is
// pointed (which chat, which conversation), and Crowbar is the sole writer of
// that, so it cannot drift.
type runnerRow struct {
	ID                  string `gorm:"primaryKey"`
	WorkspaceID         string `gorm:"index"`
	ProviderID          string
	TerminalSession     string
	CurrentChatID       string `gorm:"index"`
	CurrentSession      string `gorm:"index"`
	CurrentSessionSince time.Time
	StartedAt           time.Time
}

func (runnerRow) TableName() string {
	return "agent_runners"
}

// conversationRow is APPEND-ONLY history: every conversation a chat has ever
// hosted. It replaces AgentSegment, minus everything that described a process
// (no status, no PTY, no runner id). Never updated, never deleted — except by
// the chat delete cascade (ForgetChat).
//
// It deliberately OUTLIVES the process that created it: a dormant chat must stay
// resumable, and its old conversations must still be recognisable when a
// provider re-announces one on a later /resume. Append-only history cannot drift
// from reality; only live state can — which is exactly why this is safe to
// persist while the runner's liveness is not.
type conversationRow struct {
	ChatID      string `gorm:"primaryKey;index"`
	SessionID   string `gorm:"primaryKey;index"`
	WorkspaceID string `gorm:"index"`
	ProviderID  string
	FirstSeenAt time.Time
}

func (conversationRow) TableName() string {
	return "agent_chat_conversations"
}

// toRunner maps a live row back to the aggregate's shape. ExitedAt is always nil
// by construction: an exited runner has no row (that is the whole point), so a
// row can only ever describe a runner the model believes is still running.
func (r runnerRow) toRunner() domain.AgentRunner {
	return domain.AgentRunner{
		ID:                  r.ID,
		WorkspaceID:         r.WorkspaceID,
		ProviderID:          r.ProviderID,
		TerminalSession:     r.TerminalSession,
		CurrentChatID:       r.CurrentChatID,
		CurrentSession:      r.CurrentSession,
		CurrentSessionSince: r.CurrentSessionSince,
		StartedAt:           r.StartedAt,
	}
}

func (c conversationRow) toConversation() domain.ChatConversation {
	return domain.ChatConversation{
		ChatID:      c.ChatID,
		ProviderID:  c.ProviderID,
		SessionID:   c.SessionID,
		FirstSeenAt: c.FirstSeenAt,
	}
}
