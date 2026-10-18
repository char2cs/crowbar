package models

import "time"

// Runner is ONE live vendor-CLI process. Its ID is the crowbarSegmentID Crowbar
// mints at spawn and passes to every hook, so a hook can always name its runner.
//
// It deliberately has NO status field: the process is the SOLE authority on
// whether it is alive, and a runner is held only while it is.
type Runner struct {
	ID          string `json:"id"` // == crowbarSegmentID
	WorkspaceID string `json:"workspaceId"`
	ProviderID  string `json:"providerId"`
	// TerminalSession is its PTY — identity AND heartbeat — and is EMPTY for a
	// runner whose process is an api connection instead (runner/apirunner.go).
	// Every reader treats "" as "no terminal", never as missing data.
	TerminalSession string `json:"terminalSessionId"`

	// CurrentChatID is the chat the runner was started on; it never moves.
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

	// StartedAt is when the runner was recorded started on its chat.
	StartedAt time.Time `json:"startedAt"`
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
