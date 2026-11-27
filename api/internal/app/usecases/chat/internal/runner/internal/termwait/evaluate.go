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
	if busy && screen.matched.Waiting && d.choicePending(ctx, runner.CurrentChatID) {
		return domain.AgentTerminalWait{}, screen, ends
	}
	return screen.matched, screen, ends
}

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
		next.matched = domain.AgentTerminalWait{Waiting: true, Kind: prompt.Kind, Answers: waitAnswers(prompt)}
	}
	if d.deps.Notices != nil {
		if notice, ok := d.deps.Notices.MatchTerminalNotice(ctx, runner.ProviderID, text); ok {
			next.notice = notice
		}
	}
	return next
}

func waitAnswers(prompt agents.TerminalPrompt) []domain.AgentTerminalAnswer {
	var out []domain.AgentTerminalAnswer
	for _, a := range prompt.Answers {
		out = append(out, domain.AgentTerminalAnswer{ID: a.ID, Label: a.Label, Keys: a.Keys})
	}
	return out
}
