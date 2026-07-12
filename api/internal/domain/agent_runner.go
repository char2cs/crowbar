package domain

import "time"

// AgentRunner is ONE live vendor-CLI process in ONE PTY — the thing that
// actually moves between chats when the user types /clear or /resume inside the
// CLI. Its ID is the crowbarSegmentID Crowbar mints at spawn and passes to every
// hook, so a hook can always name its runner.
//
// It deliberately has NO status field. The PTY is the SOLE authority on whether
// this process is alive (spec §2): two authorities on liveness always drift, and
// that drift is exactly what let a segment read "ended" while its CLI was still
// running. ExitedAt is an audit tombstone, never a liveness check — ask the
// chat_liveness projection (which drops the row on exit), or the terminal engine.
//
// What IS durable here is PLACEMENT — which chat, which conversation. Crowbar is
// its only writer, so it cannot drift. Persisting it is what makes a conversation
// switch a single atomic write instead of a torn cross-aggregate one.
type AgentRunner struct {
	ID              string `json:"id"` // == crowbarSegmentID
	WorkspaceID     string `json:"workspaceId"`
	ProviderID      string `json:"providerId"`
	TerminalSession string `json:"terminalSessionId"` // its PTY: identity AND heartbeat

	// CurrentChatID is always set (invariant I1). CurrentSession is empty only
	// between spawn and the provider's first session announcement.
	CurrentChatID  string `json:"currentChatId"`
	CurrentSession string `json:"currentSessionId,omitempty"`

	// CurrentSessionSince is when CurrentSession was bound to this runner — the
	// moment the CONVERSATION opened, which is NOT the moment the runner spawned:
	// a long-lived CLI opens conversations hours after StartedAt. It is zero
	// exactly while CurrentSession is empty (spawned, nothing announced yet). The
	// conversation projection stamps FirstSeenAt from it, so history orders by
	// when each conversation actually opened rather than by whose runner started
	// first — two runners writing into one chat would otherwise invert it.
	CurrentSessionSince time.Time `json:"currentSessionSince,omitzero"`

	StartedAt time.Time  `json:"startedAt"`
	ExitedAt  *time.Time `json:"exitedAt,omitempty"` // audit only — NOT a liveness flag
}

// ChatConversation is one conversation a chat has hosted. Append-only history,
// projected from runner events — NOT chat state. It is what AgentSegment really
// was, minus everything that described a process (no status, no PTY, no runner
// id). History cannot drift from reality; only live state can. That is why this
// is safe to persist while the runner's liveness is not.
type ChatConversation struct {
	ChatID      string    `json:"chatId"`
	ProviderID  string    `json:"providerId"`
	SessionID   string    `json:"sessionId"`
	FirstSeenAt time.Time `json:"firstSeenAt"`
}
