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
	// RecordTurn appends one turn to the chat's conversation record, keyed by the
	// hook delivery so a relay retry is absorbed rather than duplicated.
	RecordTurn(
		ctx context.Context,
		chat domain.Chat,
		providerID, runnerID, sessionID string,
		role, text, effort string,
	) error
	// AppendRunnerTurn records a turn and refreshes the chat's activity clock.
	AppendRunnerTurn(
		ctx context.Context,
		chat domain.Chat,
		providerID, runnerID, sessionID string,
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

// Runners is what a hook needs from the runner lifecycle: the placement half of
// a session_start, and the two prompt-journal transitions a user_prompt confirms.
//
// NOTHING here may take the spawn gate. SwitchProvider holds it while parked on a
// turn only this package can release, so a hook that waited on it would deadlock
// against the very switch waiting on the hook.
type Runners interface {
	// HandleSessionStart applies the placement a CLI has already performed: a
	// /clear or /resume inside the TUI moves the runner, and Crowbar is told after
	// the fact.
	HandleSessionStart(
		ctx context.Context,
		runner engineagents.Runner,
		ev engineagents.CanonicalEvent,
	) error
	// ConfirmPromptAccepted closes the at-most-once journal entry for a React
	// submission the CLI has now echoed back as its own user prompt.
	ConfirmPromptAccepted(
		ctx context.Context,
		chat domain.Chat,
		runner engineagents.Runner,
		message string,
	) error
	// ReconcilePendingPromptFromLedger settles a delivery whose outcome the journal
	// could not observe, using what the chat's ledger now shows.
	ReconcilePendingPromptFromLedger(
		ctx context.Context,
		chat domain.Chat,
	) error
}
