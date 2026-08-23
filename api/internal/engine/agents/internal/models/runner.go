package models

// Runner and ChatConversation are the engine's own model of a live vendor CLI and the
// conversations it has hosted.
//
// They live here rather than in domain/ because the engine owns the runner lifecycle
// (design spec 3.1): 11 of Runner's 13 fields describe the PROCESS — its PTY, its
// native conversation, what it was launched with — and only WorkspaceID and
// CurrentChatID are Crowbar's. They are in models rather than in the agents package
// itself so the runner store, which is a child of agents, can name them without an
// import cycle.

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
type Runner struct {
	ID              string `json:"id"` // == crowbarSegmentID
	WorkspaceID     string `json:"workspaceId"`
	ProviderID      string `json:"providerId"`
	TerminalSession string `json:"terminalSessionId"` // its PTY: identity AND heartbeat

	// CurrentChatID is set while the runner is PLACED — which is its whole life, bar
	// one case: Crowbar has taken it off a chat it is being removed from (an eviction,
	// the outgoing side of a switch, a chat deleted under it) and it is still dying.
	// A displaced runner is pointed at nothing, so nothing can be written on its behalf
	// and no chat can be handed to it — while its process, which we do not command,
	// finishes falling over. CurrentSession is additionally empty between spawn and the
	// provider's first conversation announcement.
	CurrentChatID  string `json:"currentChatId,omitempty"`
	CurrentSession string `json:"currentSessionId,omitempty"`
	// LaunchSessionID is the native conversation Crowbar explicitly asked this
	// process to resume at launch. It persists the launch intent across the gap
	// before (and after) session_start, so a React prompt can restart the TUI as a
	// resume even when the resumed conversation's ledger turns predate this
	// runner's CurrentSessionSince timestamp.
	LaunchSessionID string `json:"launchSessionId,omitempty"`
	// LaunchModel and LaunchEffort are the model and reasoning effort Crowbar
	// LAUNCHED this process with — the same persisted-launch-intent pattern
	// LaunchSessionID above serves, for the same reason: the answer cannot be
	// recovered later from anything else.
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
	// CurrentSessionResumable is Crowbar's provider-neutral knowledge that the
	// current binding names an existing native conversation. handleSessionStart
	// derives it solely from append-only session history, including a user-issued
	// TUI /resume; it never depends on provider-specific source labels.
	CurrentSessionResumable bool `json:"currentSessionResumable,omitempty"`

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
