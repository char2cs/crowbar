package termwait

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func (d *detector) evaluate(
	ctx context.Context,
	runner agents.Runner,
	prev screenCache,
) (domain.AgentTerminalWait, screenCache, bool) {
	if runner.TerminalSession == "" {
		return domain.AgentTerminalWait{}, screenCache{}, false
	}

	chat, err := d.deps.Chats.GetChat(ctx, runner.CurrentChatID)
	if err != nil {
		return domain.AgentTerminalWait{}, prev, false
	}

	if chat.Working {
		if d.providerSaysItIsIdle(ctx, runner) {
			return domain.AgentTerminalWait{}, prev, false
		}
		if d.abandonedMessage(ctx, runner) {
			return domain.AgentTerminalWait{}, prev, false
		}

		screen := d.readScreen(ctx, runner, prev)
		return domain.AgentTerminalWait{}, screen, d.stalled(ctx, runner, &screen)
	}

	pending, err := d.deps.Choices.PendingChoices(ctx, runner.CurrentChatID)
	if err != nil {
		return domain.AgentTerminalWait{}, prev, false
	}
	if len(pending) > 0 {
		return domain.AgentTerminalWait{}, prev, false
	}

	screen := d.readScreen(ctx, runner, prev)
	d.settleDelivery(ctx, runner, &screen)
	return screen.matched, screen, false
}

func (d *detector) settleDelivery(
	ctx context.Context,
	runner agents.Runner,
	screen *screenCache,
) {
	if screen.settled || d.deps.Deliveries == nil {
		return
	}
	if screen.matched.Waiting {
		return
	}
	if screen.gen == 0 || screen.since.IsZero() {
		return
	}
	if d.now().Sub(screen.since) < d.deliveryQuiet() {
		return
	}
	delivery, ok := d.deps.Deliveries.PendingDelivery(ctx, runner.CurrentChatID)

	if !ok || delivery.RunnerID == "" || delivery.RunnerID != runner.ID {
		return
	}
	// And the DELIVERY's own age, not just the screen's.
	//
	// The screen clock above measures how long this PTY has drawn nothing, which
	// for an api-transport chat (codex) is the wrong question entirely: the PTY
	// beside that connection is a disconnected companion driving an unrelated
	// conversation, so it draws nothing for as long as the chat sits idle. Its
	// quiet window is therefore ALREADY hours old when a prompt arrives, and the
	// grace period this timeout exists to give — thirty seconds for the provider
	// to produce a turn — collapsed to zero: the next sweep, up to two seconds
	// later, retired the delivery outright.
	//
	// Measured live on a fresh codex chat: a delivery journalled at 14:14:59.535Z
	// was logged "produced no turn and was settled" inside the same second, while
	// the provider's own turn was still on its way (it landed 910ms later). The
	// prompt survived only because the ledger beat the client's own reaction to
	// the retirement — a race, and one an idle chat loses more often, since the
	// first prompt after a gap is exactly the slow one (session resume, cold
	// model). Losing it discards the user's typed text, which at that moment
	// exists nowhere else (see settle.go).
	if d.now().Sub(delivery.CreatedAt) < d.deliveryQuiet() {
		return
	}
	retired, err := d.deps.Deliveries.SettleDelivery(ctx, runner.CurrentChatID, delivery.RequestID)
	if err != nil || !retired {
		return
	}
	screen.settled = true
}

func (d *detector) readScreen(
	ctx context.Context,
	runner agents.Runner,
	prev screenCache,
) screenCache {
	fresh := prev.session != runner.TerminalSession
	since := prev.gen
	if fresh {
		since = 0
	}

	text, gen, changed := d.deps.Screens.Screen(runner.TerminalSession, since)
	if !changed {
		if gen == 0 || fresh {
			return screenCache{session: runner.TerminalSession}
		}

		carried := prev
		carried.gen = gen
		return carried
	}

	if !fresh && prev.text == text && !prev.since.IsZero() {
		carried := prev
		carried.gen = gen
		return carried
	}

	next := screenCache{session: runner.TerminalSession, gen: gen, text: text, since: d.now()}
	if prompt, ok := d.deps.Prompts.MatchTerminalPrompt(ctx, runner.ProviderID, text); ok {
		next.matched = domain.AgentTerminalWait{Waiting: true, Kind: prompt.Kind}
	}
	if d.deps.Notices != nil {
		if notice, ok := d.deps.Notices.MatchTerminalNotice(ctx, runner.ProviderID, text); ok {
			next.notice = notice
		}
	}
	return next
}

func (d *detector) stalled(
	ctx context.Context,
	runner agents.Runner,
	screen *screenCache,
) bool {
	if screen.fired {
		return false
	}

	if d.deps.Notices == nil || d.deps.Work == nil || d.deps.OnStall == nil {
		return false
	}

	if screen.gen == 0 || screen.since.IsZero() {
		return false
	}

	if !screen.notice.EndsTurn {
		return false
	}

	if d.now().Sub(screen.since) < d.stallQuiet() {
		return false
	}

	pending, err := d.deps.Choices.PendingChoices(ctx, runner.CurrentChatID)
	if err != nil || len(pending) > 0 {
		return false
	}

	open, err := d.deps.Work.OpenWork(ctx, runner.CurrentChatID)
	if err != nil || open {
		return false
	}

	screen.fired = true
	return true
}

func (d *detector) abandonedMessage(ctx context.Context, runner agents.Runner) bool {
	if d.deps.Messages == nil {
		return false
	}
	// A turn riding a connection Crowbar still holds is not silent because it
	// died. Codex reasons for well over this window between tool calls on a
	// long task — measured live at 31s of complete quiet in the middle of a
	// security review, with 337 more events still to come — and its shell
	// commands finish in milliseconds, so OpenWork vouches for almost none of
	// it (148 of 149 sweeps read open_work=false). This detector fired, the
	// turn was abandoned mid-answer and the spinner went dark while the CLI
	// carried on. Losing the connection is reconciled directly instead — see
	// runner/connloss.go.
	if d.deps.Liveness != nil && d.deps.Liveness.HasLiveAPIConnection(runner.ID) {
		return false
	}
	since, ok := d.deps.Messages.UnfinishedSince(runner.CurrentChatID)
	if !ok || since.IsZero() {
		return false
	}
	if d.now().Sub(since) < d.messageQuiet() {
		return false
	}
	pending, err := d.deps.Choices.PendingChoices(ctx, runner.CurrentChatID)
	if err != nil || len(pending) > 0 {
		return false
	}
	if d.deps.Work != nil {
		open, err := d.deps.Work.OpenWork(ctx, runner.CurrentChatID)
		if err != nil || open {
			return false
		}
	}
	closed, err := d.deps.Messages.AbandonMessage(ctx, runner.CurrentChatID)
	if err != nil {
		return false
	}
	return closed
}

// providerSaysItIsIdle closes a turn the provider itself has reported finished
// and that nothing else ever closed.
//
// It is tried BEFORE the two screen-derived detectors because it is the only one
// of the three that is authoritative rather than heuristic: the provider said so.
// The others infer it from a notice sitting unchanged on a PTY for two minutes,
// or from a half-written message going quiet — and a turn that only reasoned
// produces neither, which is exactly the turn that used to hang forever.
//
// Deliberately NOT gated on OpenWork, unlike those two. A tool call or subagent
// still open in Crowbar's ledger while the provider reports idle is a STALE
// record, not live work — it is the provider that knows, and AbandonMessage
// zeroes the async-work level as it closes. Gating on it would have preserved
// exactly the stuck spinner this exists to clear.
func (d *detector) providerSaysItIsIdle(ctx context.Context, runner agents.Runner) bool {
	if d.deps.Idle == nil || d.deps.Messages == nil {
		return false
	}
	since, ok := d.deps.Idle.ProviderIdleSince(runner.CurrentChatID)
	if !ok || since.IsZero() {
		return false
	}
	// The report lands microseconds before an ordinary turn close, so the wait is
	// what separates "this turn is ending normally" from "nothing is going to end
	// it". A close clears the latch, so a healthy turn never reaches here at all.
	if d.now().Sub(since) < d.idleQuiet() {
		return false
	}
	// A chat holding a prompt for a human is not stranded, whatever the provider
	// says about its own idleness: the person is the one being waited on.
	pending, err := d.deps.Choices.PendingChoices(ctx, runner.CurrentChatID)
	if err != nil || len(pending) > 0 {
		return false
	}
	closed, err := d.deps.Messages.AbandonMessage(ctx, runner.CurrentChatID)
	if err != nil {
		return false
	}
	return closed
}
