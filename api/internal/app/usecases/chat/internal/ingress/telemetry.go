package ingress

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// RecordTelemetry records a usage report runner sent over channel. It is no
// conversation event, so it is never judged by session.
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

// ForgetTelemetry drops chatID's report once another provider took the chat over:
// it describes the CLI that left, and the incoming one may never report at all.
func (r *Recorder) ForgetTelemetry(ctx context.Context, chatID string) {
	r.telemetry.Forget(ctx, chatID)
	r.frames.TelemetryGone(chatID)
}

// Telemetry is the chat provider's last usage report.
func (r *Recorder) Telemetry(chatID string) (engineagents.Telemetry, bool) {
	return r.telemetry.Get(chatID)
}
