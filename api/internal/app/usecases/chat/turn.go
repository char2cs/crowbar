package chat

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/transcript"
	"github.com/char2cs/crowbar/api/internal/domain"
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

	// PendingChoiceSets lists every chat's unresolved prompts, so a socket that
	// has just connected can be told what it missed.
	PendingChoiceSets(ctx context.Context) ([]ChoiceSet, error)

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

// ChatActivity is one page of what the agent DID: the messages it is shown
// around, the tool calls, the subagents, the interruptions and the questions.
type ChatActivity struct {
	Turns         []domain.ActivityTurn
	ToolCalls     []domain.ActivityToolCall
	Subagents     []domain.ActivitySubagent
	Interruptions []domain.ActivityInterruption
	Choices       []domain.ActivityChoice
}

// ChoiceSet is one chat's pending prompts and which of them a relay is holding.
type ChoiceSet struct {
	ChatID      string
	WorkspaceID string
	Choices     []domain.ActivityChoice
	Answerable  []string
}

// HookReply is what a placed hook's sender is answered with.
type HookReply = ingress.Reply

// ErrToolPayloadGone is a tool call's payload that was never stored or is
// stored no more.
var ErrToolPayloadGone = fmt.Errorf("agent: tool payload is no longer available: %w", apperr.ErrNotFound)

// maxActivityPage caps a page of activity turns and tool calls.
const maxActivityPage = 500

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

// ReadActivity returns one page of the chat's record: the turns and tool
// calls after the cursor, else the newest, with every subagent, interruption
// and choice.
func (u *Usecase) ReadActivity(
	ctx context.Context,
	chatID string,
	after int64,
	limit int,
) (ChatActivity, error) {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: chat: %w", err)
	}
	act, err := u.transcript.Activity(ctx, chatID)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: %w", err)
	}
	if limit <= 0 || limit > maxActivityPage {
		limit = maxActivityPage
	}
	choices := make([]domain.ActivityChoice, 0, len(act.Choices))
	for _, c := range act.Choices {
		choices = append(choices, c.ActivityChoice)
	}
	return ChatActivity{
		Turns:         pageBySeq(act.Turns, after, limit, func(t domain.ActivityTurn) int64 { return t.Seq }),
		ToolCalls:     pageBySeq(act.ToolCalls, after, limit, func(c domain.ActivityToolCall) int64 { return c.Seq }),
		Subagents:     act.Subagents,
		Interruptions: act.Interruptions,
		Choices:       choices,
	}, nil
}

// pageBySeq is the first limit of items past after, else the newest limit,
// in Seq order.
func pageBySeq[T any](items []T, after int64, limit int, seq func(T) int64) []T {
	sorted := slices.SortedStableFunc(slices.Values(items), func(a, b T) int { return cmp.Compare(seq(a), seq(b)) })
	if after <= 0 {
		return sorted[max(len(sorted)-limit, 0):]
	}
	start := slices.IndexFunc(sorted, func(t T) bool { return seq(t) > after })
	if start < 0 {
		return []T{}
	}
	return sorted[start:min(start+limit, len(sorted))]
}

// ReadPendingChoices returns the questions the chat is currently blocked on.
func (u *Usecase) ReadPendingChoices(
	ctx context.Context,
	chatID string,
) ([]domain.ActivityChoice, error) {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return nil, fmt.Errorf("agent: read pending choices: chat: %w", err)
	}
	choices, err := u.transcript.PendingChoices(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: read pending choices: %w", err)
	}
	return choices, nil
}

// PendingChoiceSets returns every chat's unresolved prompts. Only a chat with a
// live runner has any: nothing else can still be asking.
func (u *Usecase) PendingChoiceSets(ctx context.Context) ([]ChoiceSet, error) {
	chats, err := u.ledger.OpenChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: pending choice sets: %w", err)
	}
	var sets []ChoiceSet
	for _, chatID := range chats {
		st, err := u.ledger.State(ctx, chatID)
		if err != nil {
			return nil, fmt.Errorf("agent: pending choice sets: %w", err)
		}
		choices := transcript.PendingChoicesOf(st)
		if len(choices) == 0 {
			continue
		}
		chat, err := u.chats.GetChat(ctx, chatID)
		if err != nil {
			continue
		}
		sets = append(sets, ChoiceSet{
			ChatID: chatID, WorkspaceID: chat.WorkspaceID, Choices: choices,
			Answerable: u.answers.AnswerableIDs(chatID, choices),
		})
	}
	return sets, nil
}

// ReadToolPayload returns one side — request or result — of a recorded tool call.
func (u *Usecase) ReadToolPayload(
	ctx context.Context,
	chatID, toolID, side string,
) ([]byte, error) {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return nil, fmt.Errorf("agent: read tool payload: chat: %w", err)
	}
	act, err := u.transcript.Activity(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: read tool payload: %w", err)
	}
	at := slices.IndexFunc(act.ToolCalls, func(c domain.ActivityToolCall) bool { return c.ID == toolID })
	if at < 0 {
		return nil, ErrToolPayloadGone
	}
	ref := act.ToolCalls[at].RequestRef
	if side == "result" {
		ref = act.ToolCalls[at].ResultRef
	}
	if ref == "" {
		return nil, ErrToolPayloadGone
	}
	payload, err := u.blobs.Get(ref)
	if errors.Is(err, ledgerstore.ErrBlobNotFound) {
		return nil, ErrToolPayloadGone
	}
	if err != nil {
		return nil, fmt.Errorf("agent: read tool payload: %w", err)
	}
	return payload, nil
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
