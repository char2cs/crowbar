package termwait

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// evaluate is runner's verdict and screen this sweep, and whether a notice
// that ends its work has just appeared on the screen of a chat running a turn
// or awaiting a prompt. A choice the person has yet to make holds either.
func (d *detector) evaluate(
	ctx context.Context,
	runner agents.Runner,
	prev screenCache,
) (domain.AgentTerminalWait, screenCache, bool) {
	busy, awaiting, err := d.deps.Work.ChatWork(ctx, runner.CurrentChatID)
	if err != nil {
		return domain.AgentTerminalWait{}, prev, false
	}
	if runner.TerminalSession == "" {
		return domain.AgentTerminalWait{}, screenCache{}, false
	}
	if !busy && d.choicePending(ctx, runner.CurrentChatID) {
		return domain.AgentTerminalWait{}, prev, false
	}

	screen := d.readScreen(ctx, runner, prev)
	ends := (busy || awaiting) && screen.notice.EndsTurn &&
		(screen.notice != prev.notice || screen.session != prev.session)
	if ends && busy && d.choicePending(ctx, runner.CurrentChatID) {
		return domain.AgentTerminalWait{}, prev, false
	}
	if busy {
		return domain.AgentTerminalWait{}, screen, ends
	}
	return screen.matched, screen, ends
}

// choicePending reports whether chatID holds a choice the person has yet to
// make; an unreadable answer holds it too, until the next sweep.
func (d *detector) choicePending(ctx context.Context, chatID string) bool {
	pending, err := d.deps.Choices.PendingChoices(ctx, chatID)
	return err != nil || len(pending) > 0
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

	if !fresh && prev.text == text {
		carried := prev
		carried.gen = gen
		return carried
	}

	next := screenCache{session: runner.TerminalSession, gen: gen, text: text}
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
