package termwait

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func (d *detector) Sweep(ctx context.Context, publish Publish) {
	d.sweep(ctx, publish)
}

// sweep reports whether the loop should keep ticking: true while any runner is
// live, and on a census failure (the next tick retries it).
func (d *detector) sweep(ctx context.Context, publish Publish) bool {
	runners, err := d.deps.Runners.AllLive(ctx)
	if err != nil {
		return true
	}
	d.publishAndClose(ctx, publish, runners)
	return len(runners) > 0
}

func (d *detector) publishAndClose(ctx context.Context, publish Publish, runners []agents.Runner) {
	changed, notices := d.fold(ctx, runners)

	if publish != nil {
		for _, c := range changed {
			publish(c.chatID, c.workspaceID, c.wait)
		}
	}
	if d.deps.OnNotice == nil {
		return
	}
	for _, n := range notices {
		d.deps.OnNotice(ctx, n.runner, n.notice)
	}
}

// endingNotice is a notice that ends the turn, on runner's screen.
type endingNotice struct {
	runner agents.Runner
	notice agents.TerminalNotice
}

type change struct {
	chatID      string
	workspaceID string
	wait        domain.AgentTerminalWait
}

func (d *detector) fold(
	ctx context.Context,
	runners []agents.Runner,
) ([]change, []endingNotice) {
	d.mu.RLock()
	prev := make(map[string]chatState, len(d.state))
	for k, v := range d.state {
		prev[k] = v
	}
	d.mu.RUnlock()

	next := make(map[string]chatState, len(runners))
	var changed []change
	var notices []endingNotice
	for _, runner := range runners {
		chatID := runner.CurrentChatID
		if chatID == "" {
			continue
		}
		was := prev[chatID]
		verdict, screen, ends := d.evaluate(ctx, runner, was.screen)
		next[chatID] = chatState{workspaceID: runner.WorkspaceID, screen: screen, published: verdict}
		if verdict != was.published {
			changed = append(changed, change{chatID, runner.WorkspaceID, verdict})
		}
		if ends {
			notices = append(notices, endingNotice{runner: runner, notice: screen.notice})
		}
	}

	for chatID, was := range prev {
		if _, still := next[chatID]; still || !was.published.Waiting {
			continue
		}
		changed = append(changed, change{chatID: chatID, workspaceID: was.workspaceID})
	}

	d.mu.Lock()
	d.state = next
	d.mu.Unlock()
	return changed, notices
}
