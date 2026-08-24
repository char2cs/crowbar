package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// TurnUsecase is the hook ingress and everything it writes: turns opening and
// closing, tool calls, subagents, interruptions, streamed assistant messages
// and provider telemetry.
//
// IT NEVER TAKES A SPAWN GATE. A hook must never block and must never fail — by
// the time it arrives the CLI has already acted — and the switch path parks on
// an in-flight turn with no timeout, releasable only from here. A path through
// this type that waited on the spawn gate would deadlock the CLI against its own
// hook and wedge every hook after it (see inflight.Gate and awaitTurnComplete).
type TurnUsecase interface {
	// IngestHook applies one canonical hook event from a runner's CLI. It buffers
	// the event instead when the runner's row is still being persisted, so a
	// provider that fires the instant its PTY starts cannot overtake its own
	// session_start.
	IngestHook(
		ctx context.Context,
		runnerID string,
		provider string,
		canonicalEvent string,
		rawPayload []byte,
	) error

	// IngestHookDelivery is the exactly-once ingress for one RELAYED hook: it
	// dedupes the delivery, buffers it if the runner is still starting, runs its
	// effects, then durably records the completion.
	IngestHookDelivery(
		ctx context.Context,
		workspaceID, deliveryID, runnerID, provider, canonicalEvent string,
		rawPayload []byte,
	) error

	// ReadActivity pages a chat's tool calls and returns its subagents,
	// interruptions and choices alongside them.
	ReadActivity(
		ctx context.Context,
		chatID string,
		after int64,
		limit int,
	) (ChatActivity, error)

	// ReadPendingChoices lists the questions a chat's CLI is still waiting on an
	// answer to.
	ReadPendingChoices(
		ctx context.Context,
		chatID string,
	) ([]domain.ActivityChoice, error)

	// ReadToolPayload returns one tool call's stored request or result body.
	// side is "result" for the result, anything else for the request.
	ReadToolPayload(
		ctx context.Context,
		chatID, toolID, side string,
	) ([]byte, error)

	// Telemetry reports the newest provider telemetry for a chat, if its CLI has
	// sent any. It is in memory because it describes a LIVE process.
	Telemetry(
		chatID string,
	) (engineagents.Telemetry, bool)

	// OpenWork reports whether the chat has a tool call or a subagent still
	// running. It is the second opinion the stall detector needs before it closes
	// a turn whose provider went quiet.
	OpenWork(
		ctx context.Context,
		chatID string,
	) (bool, error)

	// MatchTerminalPrompt asks a provider's descriptor whether a rendered screen
	// is one of its modal prompts. An unresolvable home or descriptor is silent
	// rather than an error: it is a detector input, not a command.
	MatchTerminalPrompt(
		ctx context.Context,
		providerID string,
		screen string,
	) (engineagents.TerminalPrompt, bool)

	// MatchTerminalNotice asks a provider's descriptor whether a rendered screen
	// carries a notice worth recording against the stalled turn it closes.
	MatchTerminalNotice(
		ctx context.Context,
		providerID string,
		screen string,
	) (engineagents.TerminalNotice, bool)
}

var _ TurnUsecase = (*Usecase)(nil)

// ChatActivity is one page of what the agent DID: the tool calls, the subagents,
// the interruptions and the questions it is blocked on.
type ChatActivity = turn.ChatActivity

// The turn: what the vendor CLI did, and what it is blocked on.
//
// Everything here is driven by a hook, and a hook is a fait accompli — by the
// time one arrives the CLI has already acted. So none of it refuses, and none of
// it blocks on a lock a user-initiated path can hold.

// IngestHook applies one canonical hook event that carries no delivery id.
func (u *Usecase) IngestHook(
	ctx context.Context,
	runnerID, provider, canonicalEvent string,
	rawPayload []byte,
) error {
	return u.turns.IngestHook(ctx, runnerID, provider, canonicalEvent, rawPayload)
}

// IngestHookDelivery is the exactly-once ingress for one relayed hook.
func (u *Usecase) IngestHookDelivery(
	ctx context.Context,
	workspaceID, deliveryID, runnerID, provider, canonicalEvent string,
	rawPayload []byte,
) error {
	return u.turns.IngestHookDelivery(
		ctx, workspaceID, deliveryID, runnerID, provider, canonicalEvent, rawPayload,
	)
}

// ReadActivity returns one page of the chat's tool calls, subagents,
// interruptions and choices.
func (u *Usecase) ReadActivity(
	ctx context.Context,
	chatID string,
	after int64,
	limit int,
) (ChatActivity, error) {
	return u.turns.ReadActivity(ctx, chatID, after, limit)
}

// ReadPendingChoices returns the questions the chat is currently blocked on.
func (u *Usecase) ReadPendingChoices(
	ctx context.Context,
	chatID string,
) ([]domain.ActivityChoice, error) {
	return u.turns.ReadPendingChoices(ctx, chatID)
}

// ReadToolPayload returns one side — request or result — of a recorded tool call.
func (u *Usecase) ReadToolPayload(
	ctx context.Context,
	chatID, toolID, side string,
) ([]byte, error) {
	return u.turns.ReadToolPayload(ctx, chatID, toolID, side)
}

// Telemetry returns the chat provider's last usage report. ok is false when no
// provider has reported for the chat in this process.
func (u *Usecase) Telemetry(chatID string) (engineagents.Telemetry, bool) {
	return u.turns.Telemetry(chatID)
}

// OpenWork reports whether the chat has work in flight, from the authoritative
// process-local mirror rather than the asynchronous projection.
func (u *Usecase) OpenWork(ctx context.Context, chatID string) (bool, error) {
	return u.turns.OpenWork(ctx, chatID)
}

// MatchTerminalPrompt asks a provider's descriptor whether a rendered screen is
// one of the modal prompts it declares.
func (u *Usecase) MatchTerminalPrompt(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalPrompt, bool) {
	return u.turns.MatchTerminalPrompt(ctx, providerID, screen)
}

// MatchTerminalNotice asks a provider's descriptor whether a rendered screen is
// one of the standing notices it declares — a usage limit, a service outage.
func (u *Usecase) MatchTerminalNotice(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalNotice, bool) {
	return u.turns.MatchTerminalNotice(ctx, providerID, screen)
}
