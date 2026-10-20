package turn

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// FailTurnOnNotice ends what a notice on runner's screen ends, which no hook
// reports: its running turn fails, or the prompts it carries settle unconsumed.
// The caller holds the chat's lock, so no hook ends that work between check
// and record.
func (t *Turns) FailTurnOnNotice(ctx context.Context, runner engineagents.Runner, notice engineagents.TerminalNotice) {
	chat, err := t.chatForRunner(ctx, runner)
	if err != nil {
		return
	}
	st, err := t.ledger.State(ctx, chat.ID)
	if err != nil || !noticeEnds(st, runner.ID) {
		note(ctx, "fail turn on terminal notice: read turn", err)
		return
	}
	// The explanation is recorded before the end: a client reads the record
	// once on that edge.
	if _, err := t.salvageUnfinished(ctx, chat, runner); err != nil {
		note(ctx, "terminal notice: salvage streamed text", err)
	}
	note(ctx, "terminal notice: abandon conversation record", t.activity.Abandon(ctx, chat.ID, time.Now()))
	if notice.Text != "" {
		note(ctx, "terminal notice: record notice", t.conversations.RecordTurn(
			ctx, chat, "", runner.ProviderID, runner.ID, t.boundSession(ctx, chat.ID, runner.ProviderID),
			domain.TurnRoleNotice, notice.Text, "",
		))
	}
	_, _, err = t.ledger.Do(ctx, chat.ID, func(st ledger.State) ([]ledger.Row, error) {
		return noticeRows(st, runner.ID, notice)
	})
	if err != nil {
		note(ctx, "fail turn on terminal notice", err)
		return
	}
	slog.InfoContext(ctx, "agent: a terminal notice ended the turn",
		"chat_id", chat.ID, "runner_id", runner.ID, "notice", notice.Kind)
}

// noticeEnds reports whether a notice on runnerID's screen ends anything: the
// turn it runs, or with none running, a prompt it carries.
func noticeEnds(st ledger.State, runnerID string) bool {
	if turn, running := st.RunningTurn(); running {
		return turn.RunnerID == runnerID
	}
	return slices.ContainsFunc(st.PendingPrompts(), func(p ledger.Prompt) bool { return p.RunnerID == runnerID })
}

// noticeRows fail the turn runnerID runs for notice, or with none running,
// settle the prompts it carries unconsumed.
func noticeRows(st ledger.State, runnerID string, notice engineagents.TerminalNotice) ([]ledger.Row, error) {
	if _, running := st.RunningTurn(); !running {
		return ledger.SettleCarried(st, runnerID, false, notice.Kind)
	}
	rows, closes, err := failRows(st, runnerID, "", notice.Kind, notice.Text)
	if !closes {
		return nil, err
	}
	return rows, err
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
