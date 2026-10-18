package models

// Runner is the engine's own model of a live vendor CLI. It is in models rather
// than in the agents package itself so the runner store, which is a child of
// agents, can name it without an import cycle.

import "time"

// AgentRunner is ONE live vendor-CLI process in ONE PTY. Its ID is the
// crowbarSegmentID Crowbar mints at spawn and passes to every hook, so a hook can
// always name its runner.
//
// It deliberately has NO status field. The PTY is the SOLE authority on whether
// this process is alive (spec §2): two authorities on liveness always drift, and
// that drift is exactly what let a segment read "ended" while its CLI was still
// running. ExitedAt is an audit tombstone, never a liveness check — ask the
// chat_liveness projection (which drops the row on exit), or the terminal engine.
//
// What IS durable here is PLACEMENT — which chat. Crowbar is its only writer, so
// it cannot drift.
type Runner struct {
	ID          string `json:"id"` // == crowbarSegmentID
	WorkspaceID string `json:"workspaceId"`
	ProviderID  string `json:"providerId"`
	// TerminalSession is its PTY — identity AND heartbeat — and is EMPTY for a
	// runner whose process is an api connection instead (runner/apirunner.go).
	// Every reader treats "" as "no terminal", never as missing data.
	TerminalSession string `json:"terminalSessionId"`

	// CurrentChatID is set while the runner is PLACED — which is its whole life, bar
	// one case: Crowbar has taken it off a chat it is being removed from (an eviction,
	// the outgoing side of a switch, a chat deleted under it) and it is still dying.
	// A displaced runner is pointed at nothing, so nothing can be written on its behalf
	// and no chat can be handed to it — while its process, which we do not command,
	// finishes falling over.
	CurrentChatID string `json:"currentChatId,omitempty"`
	// LaunchModel and LaunchEffort are the model and reasoning effort Crowbar
	// LAUNCHED this process with: the answer cannot be recovered later from
	// anything else.
	//
	// They are the ONLY authority on what this CLI is running. Neither provider
	// exposes a readable "current model", and a user may have changed it inside
	// the TUI where Crowbar cannot see, so asking the process is not an option
	// that exists. Comparing these against the chat's own AgentChat.Model/Effort
	// is what decides whether the next prompt must replace the process.
	//
	// Empty is meaningful: this runner was launched under the provider's default
	// (or before a selection was ever made), which DIFFERS from every declared
	// value — so setting a first choice, and clearing one back to the default,
	// both register as a change.
	LaunchModel  string `json:"launchModel,omitempty"`
	LaunchEffort string `json:"launchEffort,omitempty"`
	// ReportedModel and ReportedEffort are what the process itself said it
	// resolved to (a session-start payload). They are display only: an alias
	// picked at launch never equals them, so comparing them with a selection
	// would call every unchanged pick a change.
	ReportedModel  string `json:"reportedModel,omitempty"`
	ReportedEffort string `json:"reportedEffort,omitempty"`
	// LaunchPermissionLevel is the same persisted-launch-intent fact as
	// LaunchModel/LaunchEffort above, for Crowbar's own permission level —
	// never empty for a chat that has ever been seeded, unlike Model/Effort.
	LaunchPermissionLevel string `json:"launchPermissionLevel,omitempty"`

	StartedAt time.Time  `json:"startedAt"`
	ExitedAt  *time.Time `json:"exitedAt,omitempty"` // audit only — NOT a liveness flag
}

// ChatConversation is one provider session a chat's ledger has bound, in the
// shape the chat detail serves.
type ChatConversation struct {
	ChatID      string    `json:"chatId"`
	ProviderID  string    `json:"providerId"`
	SessionID   string    `json:"sessionId"`
	FirstSeenAt time.Time `json:"firstSeenAt"`
	// LastActiveAt is when the session was most recently bound to the chat.
	LastActiveAt time.Time `json:"lastActiveAt"`
}

// ChatPlacement is one provider that has been PLACED on a chat, and when a
// runner of it last arrived there. A placement is recorded the moment a runner
// is pointed at a chat, so it answers for every chat any CLI has ever been
// started on.
//
// It is history, not state: the live-runner row is deleted when its process
// dies, and this is not. That is what lets a dormant chat still say what it ran.
type ChatPlacement struct {
	ChatID     string `json:"chatId"`
	ProviderID string `json:"providerId"`
	// LastPlacedAt is when a runner of this provider most recently started on
	// the chat, the same "arrival" the live model orders candidates by.
	LastPlacedAt time.Time `json:"lastPlacedAt"`
}
