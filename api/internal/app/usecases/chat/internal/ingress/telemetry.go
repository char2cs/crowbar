package ingress

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (r *Recorder) RecordTelemetry(
	ctx context.Context, runner engineagents.Runner, agent engineagents.Agent, raw []byte,
	channel engineagents.Channel,
) error {
	chat, err := r.chatForRunner(ctx, runner)
	if err != nil {
		return err
	}
	report, err := agent.ParseTelemetry(raw, time.Now(), channel)
	if err != nil {
		return fmt.Errorf("agent: telemetry: %w: %w", problem.ErrUnprocessable, err)
	}
	if report.Empty() {
		return nil
	}
	r.telemetry.Set(ctx, chat.ID, report)
	if r.runners.TelemetryOnChatSurface(ctx, chat.ID) {
		r.frames.Telemetry(chat.ID, chat.WorkspaceID, report)
	}
	return nil
}

func (r *Recorder) ForgetTelemetry(ctx context.Context, chatID string) {
	r.telemetry.Forget(ctx, chatID)
	r.frames.TelemetryGone(chatID)
}

func (r *Recorder) Telemetry(chatID string) (engineagents.Telemetry, bool) {
	return r.telemetry.Get(chatID)
}
