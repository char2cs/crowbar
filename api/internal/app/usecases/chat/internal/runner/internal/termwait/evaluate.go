package termwait

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// evaluate is runner's verdict and screen this sweep, and whether a notice
// that ends the turn has just appeared on a busy chat's screen.
func (d *detector) evaluate(
	ctx context.Context,
	runner agents.Runner,
	prev screenCache,
) (domain.AgentTerminalWait, screenCache, bool) {
	busy, err := d.deps.Busy.ChatBusy(ctx, runner.CurrentChatID)
	if err != nil {
		return domain.AgentTerminalWait{}, prev, false
	}
	if runner.TerminalSession == "" {
		return domain.AgentTerminalWait{}, screenCache{}, false
	}
	if busy {
		screen := d.readScreen(ctx, runner, prev)
		appeared := screen.notice != prev.notice || screen.session != prev.session
		return domain.AgentTerminalWait{}, screen, screen.notice.EndsTurn && appeared
	}

	pending, err := d.deps.Choices.PendingChoices(ctx, runner.CurrentChatID)
	if err != nil {
		return domain.AgentTerminalWait{}, prev, false
	}
	if len(pending) > 0 {
		return domain.AgentTerminalWait{}, prev, false
	}

	screen := d.readScreen(ctx, runner, prev)
	return screen.matched, screen, false
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
