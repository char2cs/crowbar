package turn

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Conversations is what this package needs from the chat record: the writes a
// hook produces, and the one title a hook may set.
//
// It is declared here, by the consumer, so the two never import each other.
type Conversations interface {
	// RecordTurn appends one turn to the chat's conversation record under
	// turnID, a fresh id when it is "".
	RecordTurn(
		ctx context.Context,
		chat domain.Chat,
		turnID, providerID, runnerID, sessionID string,
		role, text, effort string,
	) error
	// AppendRunnerTurn records a turn as RecordTurn does and refreshes the
	// chat's activity clock.
	AppendRunnerTurn(
		ctx context.Context,
		chat domain.Chat,
		turnID, providerID, runnerID, sessionID string,
		role, text string,
	) error
	// RenameChat retitles under the user > agent > derived precedence. A hook only
	// ever supplies a "derived" title, which never overwrites one already set.
	RenameChat(
		ctx context.Context,
		chatID, title, source string,
	) error
	// ChatTurns is the chat's turns as the tool surface renders them, read here to
	// decide whether a resumed CLI has anything to be told about.
	ChatTurns(
		ctx context.Context,
		chatID string,
	) ([]domain.LedgerTurn, error)
}

// Runners is what a hook needs from the runner lifecycle: the runner it names,
// and the placement half of a session_start.
//
// NOTHING here may take the chat's lock: a hook is recorded under it already.
type Runners interface {
	// RunnerByID is the live runner a hook names, also while it is exiting.
	RunnerByID(runnerID string) (engineagents.Runner, bool)
	// TurnRunner runs open on the runner a hook names, held against its end,
	// and reports false without running it once that end is under way.
	TurnRunner(runnerID string, open func(engineagents.Runner) error) (opened bool, err error)
	// HandleSessionStart applies a CLI's session announcement to the chat it
	// is on: a /clear or /resume inside the TUI changes the chat's session.
	HandleSessionStart(
		ctx context.Context,
		runner engineagents.Runner,
		ev engineagents.CanonicalEvent,
	) error
	// TelemetryOnChatSurface reports whether the chat's current surface
	// carries its provider's usage report at all.
	TelemetryOnChatSurface(ctx context.Context, chatID string) bool
	// ShowingNativeView reports whether runnerID is handed over to its
	// provider's own view right now (runner.SwitchToTerminal). holdForAnswer
	// reads it to stay out of a decision the CLI is about to put on screen
	// itself — see its own comment. surfaceGated (design spec P6b tag 2) also
	// reads it, to decide which surface a per-event surfaces: list is checked
	// against.
	ShowingNativeView(runnerID string) bool
}
