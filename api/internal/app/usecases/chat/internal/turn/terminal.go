package turn

import (
	"context"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// FailTurnOnNotice records runner's running turn failed for a notice on its
// screen that ends a turn — a usage limit, an outage — which no hook reports.
func (t *Turns) FailTurnOnNotice(ctx context.Context, runner engineagents.Runner, notice engineagents.TerminalNotice) {
	chat, ok, err := t.chatForRunner(ctx, runner)
	if err != nil || !ok {
		return
	}
	ends := func(st ledger.State) bool {
		turn, running := st.RunningTurn()
		return running && turn.RunnerID == runner.ID
	}
	st, err := t.ledger.State(ctx, chat.ID)
	if err != nil || !ends(st) {
		note(ctx, "fail turn on terminal notice: read turn", err)
		return
	}
	// The explanation is recorded before the turn's end: a client reads the
	// record once on that edge.
	if _, err := t.salvageUnfinished(ctx, chat, runner); err != nil {
		note(ctx, "terminal notice: salvage streamed text", err)
	}
	note(ctx, "terminal notice: abandon conversation record", t.activity.Abandon(ctx, chat.ID, time.Now()))
	if notice.Text != "" {
		note(ctx, "terminal notice: record notice", t.conversations.RecordTurn(
			ctx, chat, runner.ProviderID, runner.ID, t.boundSession(ctx, chat.ID, runner.ProviderID),
			domain.TurnRoleNotice, notice.Text, "",
		))
	}
	if err := t.failTurn(ctx, chat.ID, runner.ID, ends, notice.Kind, notice.Text); err != nil {
		note(ctx, "fail turn on terminal notice", err)
		return
	}
	slog.InfoContext(ctx, "agent: a terminal notice ended the turn",
		"chat_id", chat.ID, "runner_id", runner.ID, "notice", notice.Kind)
}

// MatchTerminalNotice asks runner's descriptor whether screen shows one of
// the notices it declares.
func (t *Turns) MatchTerminalNotice(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalNotice, bool) {
	descriptor, ok := t.descriptor(ctx, providerID)
	if !ok {
		return engineagents.TerminalNotice{}, false
	}
	matcher, ok := descriptor.(engineagents.NoticeMatcher)
	if !ok {
		return engineagents.TerminalNotice{}, false
	}
	return matcher.MatchTerminalNotice(screen)
}

// MatchTerminalPrompt asks a provider's descriptor whether a rendered screen
// is one of the modal prompts it declares. Every unresolvable input is a
// silent no: the sweep must keep classifying the screens it can.
func (t *Turns) MatchTerminalPrompt(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalPrompt, bool) {
	descriptor, ok := t.descriptor(ctx, providerID)
	if !ok {
		return engineagents.TerminalPrompt{}, false
	}
	return descriptor.MatchTerminalPrompt(screen)
}

func (t *Turns) descriptor(ctx context.Context, providerID string) (engineagents.Agent, bool) {
	home, err := t.home()
	if err != nil {
		return nil, false
	}
	descriptor, err := t.agents.Get(ctx, home, providerID)
	return descriptor, err == nil
}
