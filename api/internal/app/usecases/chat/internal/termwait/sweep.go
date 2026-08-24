package termwait

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func (d *detector) Sweep(ctx context.Context, publish Publish) {
	runners, err := d.deps.Runners.AllLive(ctx)
	if err != nil {
		return
	}

	changed, stalls := d.fold(ctx, runners)

	if publish != nil {
		for _, c := range changed {
			publish(c.chatID, c.workspaceID, c.wait)
		}
	}
	if d.deps.OnStall == nil {
		return
	}
	for _, s := range stalls {
		d.deps.OnStall(ctx, s)
	}
}

type change struct {
	chatID      string
	workspaceID string
	wait        domain.AgentTerminalWait
}

func (d *detector) fold(
	ctx context.Context,
	runners []agents.Runner,
) ([]change, []Stall) {
	d.mu.RLock()
	prev := make(map[string]chatState, len(d.state))
	for k, v := range d.state {
		prev[k] = v
	}
	d.mu.RUnlock()

	next := make(map[string]chatState, len(runners))
	var changed []change
	var stalls []Stall
	for _, runner := range runners {
		chatID := runner.CurrentChatID
		if chatID == "" {
			continue
		}
		was := prev[chatID]
		verdict, screen, stalled := d.evaluate(ctx, runner, was.screen)
		next[chatID] = chatState{workspaceID: runner.WorkspaceID, screen: screen, published: verdict}
		if verdict != was.published {
			changed = append(changed, change{chatID, runner.WorkspaceID, verdict})
		}
		if stalled {
			stalls = append(stalls, Stall{
				ChatID:      chatID,
				WorkspaceID: runner.WorkspaceID,
				ProviderID:  runner.ProviderID,
				RunnerID:    runner.ID,
				SessionID:   runner.TerminalSession,
				Notice:      screen.notice,
			})
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
	return changed, stalls
}
