package chat

import (
	"context"
	"fmt"
	"io"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type TurnUsecase interface {
	Hook(
		ctx context.Context,
		runnerID, provider, event string,
		raw []byte,
		channel engineagents.Channel,
	) (HookReply, error)

	DeadLetterCount(ctx context.Context) (int, error)

	Telemetry(
		chatID string,
	) (engineagents.Telemetry, bool)
	TelemetryOnChatSurface(
		ctx context.Context,
		chatID string,
	) bool

	MatchTerminalPrompt(
		ctx context.Context,
		providerID string,
		screen string,
	) (engineagents.TerminalPrompt, bool)

	MatchTerminalNotice(
		ctx context.Context,
		providerID string,
		screen string,
	) (engineagents.TerminalNotice, bool)

	UploadAttachment(
		ctx context.Context,
		chatID string,
		in UploadAttachmentInput,
	) (StoredAttachment, error)

	ReadAttachment(
		ctx context.Context,
		chatID, fileName string,
	) ([]byte, string, error)

	BeginAttachmentUpload(ctx context.Context, chatID, id, fileName string, size int64) (int64, error)
	AppendAttachmentUpload(ctx context.Context, chatID, id string, offset int64, chunk io.Reader) (int64, error)
	CompleteAttachmentUpload(ctx context.Context, chatID, id, fileName string, size int64) (StoredAttachment, error)
	AbortAttachmentUpload(ctx context.Context, chatID, id string) error
}

var _ TurnUsecase = (*Usecase)(nil)

type HookReply = ingress.Reply

func (u *Usecase) Hook(
	ctx context.Context,
	runnerID, provider, event string,
	raw []byte,
	channel engineagents.Channel,
) (HookReply, error) {
	return u.ingress.Hook(ctx, runnerID, provider, event, raw, channel)
}

func (u *Usecase) DeadLetterCount(ctx context.Context) (int, error) {
	n, err := u.ingress.Refusals(ctx)
	if err != nil {
		return 0, fmt.Errorf("agent: dead letter count: %w", err)
	}
	return n, nil
}

func (u *Usecase) Telemetry(chatID string) (engineagents.Telemetry, bool) {
	return u.recorder.Telemetry(chatID)
}

func (u *Usecase) MatchTerminalPrompt(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalPrompt, bool) {
	return u.recorder.MatchTerminalPrompt(ctx, providerID, screen)
}

func (u *Usecase) MatchTerminalNotice(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalNotice, bool) {
	return u.recorder.MatchTerminalNotice(ctx, providerID, screen)
}
