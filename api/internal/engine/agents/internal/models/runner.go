package models

import "time"

// Runner is ONE live vendor-CLI process; its ID is the crowbarSegmentID passed to
// every hook. It has NO status field: the process is the sole authority on whether
// it is alive, and a runner is held only while it is.
type Runner struct {
	ID          string `json:"id"` // == crowbarSegmentID
	WorkspaceID string `json:"workspaceId"`
	ProviderID  string `json:"providerId"`
	// TerminalSession is its PTY (identity AND heartbeat), EMPTY for an api-connection
	// runner. Readers treat "" as "no terminal", never as missing data.
	TerminalSession string `json:"terminalSessionId"`

	// CurrentChatID is the chat the runner was started on; it never moves.
	CurrentChatID string `json:"currentChatId,omitempty"`
	// LaunchModel and LaunchEffort are what Crowbar LAUNCHED this process with, the
	// only authority on what it runs (CLIs expose no readable current model); the
	// chat's selection is compared to them. Empty means the provider default.
	LaunchModel  string `json:"launchModel,omitempty"`
	LaunchEffort string `json:"launchEffort,omitempty"`
	// ReportedModel and ReportedEffort are what the process said it resolved to.
	// Display only: a launch alias never equals them, so they must not be compared.
	ReportedModel  string `json:"reportedModel,omitempty"`
	ReportedEffort string `json:"reportedEffort,omitempty"`
	// LaunchPermissionLevel is the launch-time permission level; unlike
	// LaunchModel it is never empty once the chat has been seeded.
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
