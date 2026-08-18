package termwait

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Sweep re-evaluates every chat holding a live runner, publishes the WAIT
// verdicts that CHANGED, and reports the turns a provider has abandoned.
//
// The census is the live-runner read model, and it is also the pruner: a chat that
// is not in it has no process, so it cannot be blocked on one. Its state is
// dropped, and if it was reported waiting a clearing verdict is published — which
// is how "the CLI died while parked on a dialog" reaches the client instead of
// leaving a banner up over nothing.
//
// A chat whose runner is DISPLACED (placed on no chat while its process dies) has
// an empty CurrentChatID and is skipped: there is no chat to say anything about.
func (d *detector) Sweep(ctx context.Context, publish Publish) {
	runners, err := d.deps.Runners.AllLive(ctx)
	if err != nil {
		// The census is the one read with no safe fallback: an empty list would
		// clear every standing verdict and take every banner down, so a failed
		// read leaves the whole picture exactly as it was.
		return
	}

	changed, stalls := d.fold(ctx, runners)
	// Both handed on OUTSIDE the lock. Publish reaches the hub, which fans out to
	// every connected client, and the stall callback issues repository commands —
	// holding the detector's lock across either would make an unrelated slow
	// socket, or a slow disk, block the next tick.
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

// change is one chat whose verdict moved.
type change struct {
	chatID      string
	workspaceID string
	wait        domain.AgentTerminalWait
}

// fold runs the gates for every live runner and swaps in the new state map,
// returning the chats whose published verdict moved and the chats whose turn is
// to be closed.
func (d *detector) fold(
	ctx context.Context,
	runners []domain.AgentRunner,
) ([]change, []Stall) {
	// Evaluated against a SNAPSHOT of the previous state rather than under the
	// lock: the gates make repository and terminal calls, and none of those may
	// run with the detector locked.
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

	// Chats that left the census. A standing "waiting" verdict on one of them is a
	// banner over a process that no longer exists, so it is cleared explicitly
	// rather than merely forgotten.
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
