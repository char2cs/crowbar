package turn

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"

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
// a session_start, and the refusal of a steered prompt its turn never delivered.
//
// NOTHING here may take the spawn gate. SwitchProvider holds it while parked on a
// turn only this package can release, so a hook that waited on it would deadlock
// against the very switch waiting on the hook.
type Runners interface {
	// ConfirmLaunch records that runnerID's CLI reported something: whatever
	// session it was launched to resume, it accepted.
	ConfirmLaunch(runnerID string)
	// HandleSessionStart applies the placement a CLI has already performed: a
	// /clear or /resume inside the TUI moves the runner, and Crowbar is told after
	// the fact.
	HandleSessionStart(
		ctx context.Context,
		runner engineagents.Runner,
		ev engineagents.CanonicalEvent,
	) error
	// HasLiveAPIConnection reports whether runnerID's channel is an api
	// connection right now (whose originated-session record is then current).
	HasLiveAPIConnection(runnerID string) bool
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
	// OriginatedSession reports whether runnerID's own api connection produced
	// sessionID, or is producing one right now. False for a hooks-only runner,
	// which has no connection that could.
	OriginatedSession(runnerID, sessionID string) bool
	// RefuseSteered fails a parked prompt the CLI provably never received, so
	// the client keeps its text and may retry.
	RefuseSteered(ctx context.Context, s inflight.Steered)
}
