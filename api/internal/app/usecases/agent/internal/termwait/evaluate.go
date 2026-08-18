package termwait

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// evaluate answers the four gates for ONE live runner. It returns the verdict and
// the screen cache to carry into the next tick.
//
// The order is the contract (see the package doc): liveness, then not-Working,
// then no-pending-choice, then — and only then — the screen. Every gate that can
// be answered from memory is answered before the one that cannot.
//
// A gate that FAILS TO READ is treated as "does not apply". A repository error
// here is a transient daemon condition, and the wrong response to it is to
// announce that a healthy chat is stuck: this feature's whole value is that the
// banner means something, so it stays silent whenever it is not sure.
func (d *detector) evaluate(
	ctx context.Context,
	runner domain.AgentRunner,
	prev screenCache,
) (domain.AgentTerminalWait, screenCache) {
	// Gate 1 is already half-answered: a live-runner row exists, so a process
	// does. What remains is the PTY, which is the runner's identity AND its
	// heartbeat — a runner with no session id has nothing to look at.
	if runner.TerminalSession == "" {
		return domain.AgentTerminalWait{}, screenCache{}
	}

	// Gate 2 — not Working. Read from the chat aggregate's own fold; see Chats.
	//
	// The screen cache SURVIVES gates 2 and 3: a chat goes busy and idle
	// repeatedly while its screen sits on the same content, and dropping the
	// cache here would force a re-render on the far side of every turn.
	chat, err := d.deps.Chats.GetChat(ctx, runner.CurrentChatID)
	if err != nil {
		return domain.AgentTerminalWait{}, prev
	}
	if chat.Working {
		return domain.AgentTerminalWait{}, prev
	}

	// Gate 3 — nothing outstanding that the CHAT can answer.
	//
	// ANY pending choice suppresses this, not merely an answerable one. An
	// unanswerable pending choice — the relay's window closed, the daemon
	// restarted under it — is genuinely something the user must handle at the
	// terminal, but the chat is ALREADY showing that prompt's own card and saying
	// so. A second, differently-worded banner over the top of it would be two
	// surfaces describing one prompt.
	pending, err := d.deps.Choices.PendingChoices(ctx, runner.CurrentChatID)
	if err != nil {
		return domain.AgentTerminalWait{}, prev
	}
	if len(pending) > 0 {
		return domain.AgentTerminalWait{}, prev
	}

	// Gate 4 — the screen. The only gate that costs anything, and the only one
	// that can be skipped.
	return d.matchScreen(ctx, runner, prev)
}

// matchScreen reads the PTY's visible screen and matches it against the provider's
// declared needles, re-using the previous answer when the screen has not moved.
func (d *detector) matchScreen(
	ctx context.Context,
	runner domain.AgentRunner,
	prev screenCache,
) (domain.AgentTerminalWait, screenCache) {
	// A runner replaced on the same chat brings a NEW PTY, whose generation
	// counter is its own. Comparing across the two would let a fresh screen read
	// as unchanged and inherit the dead process's verdict, so a session change
	// forces a full read by asking from generation zero.
	since := prev.gen
	if prev.session != runner.TerminalSession {
		since = 0
	}

	text, gen, changed := d.deps.Screens.Screen(runner.TerminalSession, since)
	if !changed {
		if gen == 0 {
			// Nothing to read at all: the session is gone, is a suspended
			// placeholder, or its model backend cannot render text. No evidence
			// means no claim, and the cache is dropped so a replacement PTY on
			// this chat starts from a real read.
			return domain.AgentTerminalWait{}, screenCache{session: runner.TerminalSession}
		}
		// The screen has not moved. This is the steady state for a chat parked on
		// a dialog — and for every idle chat in the daemon — so it must, and does,
		// cost one integer compare.
		return prev.matched, screenCache{
			session: runner.TerminalSession,
			gen:     gen,
			matched: prev.matched,
		}
	}

	next := screenCache{session: runner.TerminalSession, gen: gen}
	if prompt, ok := d.deps.Prompts.MatchTerminalPrompt(ctx, runner.ProviderID, text); ok {
		next.matched = domain.AgentTerminalWait{Waiting: true, Kind: prompt.Kind}
	}
	return next.matched, next
}
