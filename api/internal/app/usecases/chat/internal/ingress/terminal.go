package ingress

import (
	"context"
	"log/slog"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// FailTurnOnNotice ends what a notice on runner's screen ends and no hook
// reports: its turn fails with the notice, or its prompts settle unconsumed.
// The caller holds the chat's lock, so no hook ends that work meanwhile.
func (r *Recorder) FailTurnOnNotice(ctx context.Context, runner engineagents.Runner, notice engineagents.TerminalNotice) {
	chat, err := r.chatForRunner(ctx, runner)
	if err != nil {
		return
	}
	live := r.unrecorded(chat.ID, runner.ID)
	rows, _, err := r.ledger.Do(ctx, chat.ID, func(st ledger.State) ([]ledger.Row, error) {
		if !noticeEnds(st, runner.ID) {
			return nil, nil
		}
		return noticeRows(st, runner.ID, notice, live)
	})
	if err != nil {
		slog.WarnContext(ctx, "agent: fail turn on terminal notice", "chat_id", chat.ID, "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	r.live.forget(chat.ID, runner.ID, "")
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

// MatchTerminalNotice asks providerID's descriptor whether screen shows one of
// the notices it declares.
func (r *Recorder) MatchTerminalNotice(
	ctx context.Context, providerID, screen string,
) (engineagents.TerminalNotice, bool) {
	descriptor, ok := r.descriptor(ctx, providerID)
	if !ok {
		return engineagents.TerminalNotice{}, false
	}
	matcher, ok := descriptor.(engineagents.NoticeMatcher)
	if !ok {
		return engineagents.TerminalNotice{}, false
	}
	return matcher.MatchTerminalNotice(screen)
}

// MatchTerminalPrompt asks providerID's descriptor whether a rendered screen
// is one of the modal prompts it declares. Every unresolvable input is a
// silent no: the sweep must keep classifying the screens it can.
func (r *Recorder) MatchTerminalPrompt(
	ctx context.Context, providerID, screen string,
) (engineagents.TerminalPrompt, bool) {
	descriptor, ok := r.descriptor(ctx, providerID)
	if !ok {
		return engineagents.TerminalPrompt{}, false
	}
	return descriptor.MatchTerminalPrompt(screen)
}

func (r *Recorder) descriptor(ctx context.Context, providerID string) (engineagents.Agent, bool) {
	home, err := r.home()
	if err != nil {
		return nil, false
	}
	descriptor, err := r.agents.Get(ctx, home, providerID)
	return descriptor, err == nil
}
