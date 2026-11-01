package chat

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// TurnUsecase is the hook ingress and everything it writes: turns opening and
// closing, tool calls, subagents, interruptions, streamed assistant messages
// and provider telemetry.
//
// A hook is recorded under its chat's lock and answered only once its rows are
// in; what cannot be placed is refused and dead-lettered. A held ask waits
// with the lock released, so it never holds up the chat's other hooks.
type TurnUsecase interface {
	// Hook places one hook a runner's CLI sent over channel, and returns once
	// its rows are committed. Reply.Ask names the ask the sender is held on.
	Hook(
		ctx context.Context,
		runnerID, provider, event string,
		raw []byte,
		channel engineagents.Channel,
	) (HookReply, error)

	// DeadLetterCount is how many hooks were refused to their senders and kept
	// for inspection.
	DeadLetterCount(ctx context.Context) (int, error)

	// Telemetry reports the newest provider telemetry for a chat, if its CLI has
	// sent any. It is in memory because it describes a LIVE process.
	Telemetry(
		chatID string,
	) (engineagents.Telemetry, bool)
	// TelemetryOnChatSurface reports whether the report above can still be
	// TRUE: the store is durable, so a chat that moved to a surface whose
	// channel carries no usage would otherwise go on serving the last number
	// it earned somewhere else. See runner/capabilities.go.
	TelemetryOnChatSurface(
		ctx context.Context,
		chatID string,
	) bool

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

	// UploadAttachment stores one attachment into chatID's durable attachment
	// store, returning the logical reference written back into the message.
	UploadAttachment(
		ctx context.Context,
		chatID string,
		in UploadAttachmentInput,
	) (StoredAttachment, error)

	// ReadAttachment resolves chatID's stored attachment fileName to bytes and
	// a sniffed content type, for the asset-serving GET endpoint.
	ReadAttachment(
		ctx context.Context,
		chatID, fileName string,
	) ([]byte, string, error)
}

var _ TurnUsecase = (*Usecase)(nil)

// HookReply is what a placed hook's sender is answered with.
type HookReply = ingress.Reply

// Hook places one hook through the ingress.
func (u *Usecase) Hook(
	ctx context.Context,
	runnerID, provider, event string,
	raw []byte,
	channel engineagents.Channel,
) (HookReply, error) {
	return u.ingress.Hook(ctx, runnerID, provider, event, raw, channel)
}

// DeadLetterCount is how many hooks the ingress refused and kept.
func (u *Usecase) DeadLetterCount(ctx context.Context) (int, error) {
	n, err := u.ingress.Refusals(ctx)
	if err != nil {
		return 0, fmt.Errorf("agent: dead letter count: %w", err)
	}
	return n, nil
}

// Telemetry returns the chat provider's last usage report. ok is false when no
// provider has reported for the chat in this process.
//
// It says nothing about whether that report can still be TRUE — the store is
// durable and a chat can move to a surface whose channel carries no usage at
// all. TelemetryOnChatSurface is that question, asked separately so this one
// stays a plain read.
func (u *Usecase) Telemetry(chatID string) (engineagents.Telemetry, bool) {
	return u.recorder.Telemetry(chatID)
}

// MatchTerminalPrompt asks a provider's descriptor whether a rendered screen is
// one of the modal prompts it declares.
func (u *Usecase) MatchTerminalPrompt(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalPrompt, bool) {
	return u.recorder.MatchTerminalPrompt(ctx, providerID, screen)
}

// MatchTerminalNotice asks a provider's descriptor whether a rendered screen is
// one of the standing notices it declares — a usage limit, a service outage.
func (u *Usecase) MatchTerminalNotice(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalNotice, bool) {
	return u.recorder.MatchTerminalNotice(ctx, providerID, screen)
}
