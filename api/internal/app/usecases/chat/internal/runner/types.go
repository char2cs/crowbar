package runner

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Conversations is what starting, replacing and prompting a CLI needs from the
// chat record: what the chat has chosen, what it has said, and — when a spawn
// fails — the ability to erase the chat it was for.
//
// It is declared here, by the consumer, so the two never import each other.
type Conversations interface {
	// ChatProviderID is the provider the chat's next CLI should be, resolved from
	// its selection and the live provider order.
	ChatProviderID(
		ctx context.Context,
		chatID string,
	) (string, error)
	// ChatSelection is the model and reasoning effort pinned to the chat. create
	// says the chat is being minted right now, so there is nothing to read.
	ChatSelection(
		ctx context.Context,
		chatID string,
		create bool,
	) (engineagents.Selection, error)
	// ChatTurns is the chat's turns, read to decide whether a resumed CLI has
	// anything to be told about.
	ChatTurns(
		ctx context.Context,
		chatID string,
	) ([]domain.LedgerTurn, error)
	// AssembleConversation renders the prior exchange a replacement CLI is handed.
	// resuming says the incoming CLI will re-read the provider's own transcript, so
	// only what happened after leftAt needs restating.
	AssembleConversation(
		ctx context.Context,
		chatID string,
		resuming bool,
		leftAt time.Time,
	) (string, error)
	// ThreadContext is the prompt fragment naming the chats this one reads.
	// minting says the chat is being created now and can have no lineage yet.
	ThreadContext(
		ctx context.Context,
		chatID string,
		minting bool,
	) (string, error)
	// PurgeLocked erases a chat whose caller ALREADY holds its spawn gate. A spawn
	// that fails discards the chat it minted, from inside that gate.
	PurgeLocked(
		ctx context.Context,
		chatID string,
	) error
	// SeedPermissionLevel durably writes the current global default onto a
	// freshly created chat — see Conversations.MintChat's own use of this,
	// which SpawnChat and moveToNewChat mirror since neither can call
	// MintChat itself (both pre-generate chatID before the row exists).
	SeedPermissionLevel(
		ctx context.Context,
		chatID string,
	)
}

// Turns is what the CLI lifecycle needs from the hook ingress: the turn a switch
// must not decapitate, the startup hooks a fresh runner buffered, and the screen
// classifiers the terminal-wait sweep runs on.
//
// NOTHING here may take the spawn gate, and nothing here does: this side holds it
// while parked on a turn only the hook side can release.
type Turns interface {
	// IngestHook applies one canonical hook or api-transport event, the same
	// entrypoint the HTTP hook ingress uses — pumpAPIConn (apiconn.go) feeds an
	// api-transport connection's resolved events through here too, so ownership,
	// activity and the answer desk need no transport-specific branch.
	IngestHook(
		ctx context.Context,
		runnerID string,
		provider string,
		canonicalEvent string,
		rawPayload []byte,
	) error
	// ReplayStartupHook ingests one hook that arrived before the runner row
	// existed, now that it does.
	ReplayStartupHook(runnerID string, hook inflight.Hook)
	// AwaitTurnComplete blocks until the chat has no turn in flight, so a
	// destructive replacement never kills a CLI mid-answer.
	AwaitTurnComplete(
		ctx context.Context,
		chatID string,
	) error
	// ChatWorking reports whether the chat is busy, from the authoritative
	// process-local mirror rather than the asynchronous projection.
	ChatWorking(
		ctx context.Context,
		chatID string,
	) (bool, error)
	// RecordStop notes, durably, that a person cut chatID's in-flight turn
	// short. A no-op when the chat is idle.
	RecordStop(
		ctx context.Context,
		chatID string,
	) error
	// RecordChatSwitch notes, durably, that Crowbar itself changed chatID's
	// provider, model or effort. kind is one of
	// InterruptProviderSwitched/InterruptModelChanged/InterruptEffortChanged
	// (engine/agents); detail is the new value. The caller is the one that
	// knows whether the value actually changed — this always records.
	RecordChatSwitch(
		ctx context.Context,
		chatID, kind, detail string,
	) error
	// SetMessageDelta wires the growing-assistant-message fan-out at sweep start.
	SetMessageDelta(fn func(chatID, workspaceID, messageID, text string))
	// AbandonMessageForRunner salvages runner's own already-streamed-but-not-
	// yet-final message before closeAbandonedTurn tears its turn down — see
	// its own doc comment for why the runner must be named explicitly rather
	// than re-resolved.
	AbandonMessageForRunner(
		ctx context.Context,
		chatID string,
		runner engineagents.Runner,
	) (bool, error)

	// The four seams the terminal-wait detector reads through. They are here
	// rather than passed separately because they all belong to the hook side, and
	// splitting them would only make the detector's construction lie about that.
	termwait.Prompts
	termwait.Notices
	termwait.Work
	termwait.Messages
	// CloseStalledTurn ends a turn the screen says will never finish — a usage
	// limit, a service outage — because no hook will ever report it.
	CloseStalledTurn(ctx context.Context, stall seam.Stall)
}

// Providers is the provider table as the spawn and switch paths consult it: may
// this provider be used at all, and may its CLI call back into Crowbar.
type Providers interface {
	// RequireProviderEnabled refuses a spawn onto a provider the user switched off.
	RequireProviderEnabled(
		ctx context.Context,
		providerID string,
	) error
	// ProviderMCPEnabled reports whether the provider may use the tool surface, so
	// a spawn knows whether to render the MCP config at all.
	ProviderMCPEnabled(
		ctx context.Context,
		providerID string,
	) (bool, error)
}

// The detector reads an un-echoed React prompt through this type directly, so the
// two halves of that seam are asserted here rather than discovered at wiring time.
var _ termwait.Deliveries = (*Runners)(nil)
