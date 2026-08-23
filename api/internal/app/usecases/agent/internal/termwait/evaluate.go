package termwait

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func (d *detector) evaluate(
	ctx context.Context,
	runner domain.AgentRunner,
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
	runner domain.AgentRunner,
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
	retired, err := d.deps.Deliveries.SettleDelivery(ctx, runner.CurrentChatID, delivery.RequestID)
	if err != nil || !retired {
		return
	}
	screen.settled = true
}

func (d *detector) readScreen(
	ctx context.Context,
	runner domain.AgentRunner,
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
	runner domain.AgentRunner,
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

func (d *detector) abandonedMessage(ctx context.Context, runner domain.AgentRunner) bool {
	if d.deps.Messages == nil {
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
