package termwait

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// evaluate answers both questions for ONE live runner, from ONE screen read. It
// returns the wait verdict, the screen cache to carry into the next tick, and
// whether this chat's open turn should be closed as abandoned.
//
// The order is the contract (see the package doc): liveness, then the chat's own
// busy state, and only then the screen. Every gate that can be answered from
// memory is answered before the one that cannot.
//
// Working is the FORK, not a gate that ends the walk. An idle chat can be blocked
// on a modal; a busy one can be wedged on a turn nothing will ever close; neither
// can be the other. So the busy read chooses which question is being asked, and
// the screen read below it serves whichever one that is.
//
// A gate that FAILS TO READ is treated as "does not apply". A repository error
// here is a transient daemon condition, and the wrong response to it is either to
// announce that a healthy chat is stuck or to close a turn that is running: this
// whole mechanism's value is that its answers mean something, so it stays silent
// whenever it is not sure.
func (d *detector) evaluate(
	ctx context.Context,
	runner domain.AgentRunner,
	prev screenCache,
) (domain.AgentTerminalWait, screenCache, bool) {
	// Gate 1 is already half-answered: a live-runner row exists, so a process
	// does. What remains is the PTY, which is the runner's identity AND its
	// heartbeat — a runner with no session id has nothing to look at.
	if runner.TerminalSession == "" {
		return domain.AgentTerminalWait{}, screenCache{}, false
	}

	// Read from the chat aggregate's own fold; see Chats.
	//
	// The screen cache SURVIVES a failed read: a chat goes busy and idle
	// repeatedly while its screen sits on the same content, and dropping the
	// cache here would force a re-render on the far side of every turn.
	chat, err := d.deps.Chats.GetChat(ctx, runner.CurrentChatID)
	if err != nil {
		return domain.AgentTerminalWait{}, prev, false
	}

	if chat.Working {
		// The stall question. A busy agent is never reported waiting — the
		// spinner already says what it is doing, and a "your agent is stuck"
		// banner over the commonest state a chat is ever in would be a false
		// alarm — so the wait verdict here is unconditionally zero.
		screen := d.readScreen(ctx, runner, prev)
		return domain.AgentTerminalWait{}, screen, d.stalled(ctx, runner, &screen)
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
		return domain.AgentTerminalWait{}, prev, false
	}
	if len(pending) > 0 {
		return domain.AgentTerminalWait{}, prev, false
	}

	// Gate 4 — the screen. The only gate that costs anything, and the only one
	// that can be skipped.
	screen := d.readScreen(ctx, runner, prev)
	return screen.matched, screen, false
}

// readScreen reads the PTY's visible screen ONCE and answers everything the
// screen can say: which declared prompt it is showing, which declared notice it
// is showing, and how long it has been showing them.
//
// Both matches are made even though at most one of them can be acted on this
// tick, and that is deliberate rather than wasteful. The expensive part is
// rendering the grid to text, which happens once either way; the two matches are
// substring scans over the result. Matching only the one the current busy state
// needs would mean the cache described a question that was not asked, and the
// next tick — which may ask the other one against the same unmoved screen — would
// read an answer that was never computed.
func (d *detector) readScreen(
	ctx context.Context,
	runner domain.AgentRunner,
	prev screenCache,
) screenCache {
	// A runner replaced on the same chat brings a NEW PTY, whose generation
	// counter is its own. Comparing across the two would let a fresh screen read
	// as unchanged and inherit the dead process's verdict, so a session change
	// forces a full read by asking from generation zero.
	fresh := prev.session != runner.TerminalSession
	since := prev.gen
	if fresh {
		since = 0
	}

	text, gen, changed := d.deps.Screens.Screen(runner.TerminalSession, since)
	if !changed {
		if gen == 0 || fresh {
			// Nothing to read at all: the session is gone, is a suspended
			// placeholder, or its model backend cannot render text. No evidence
			// means no claim, and the cache is dropped so a replacement PTY on
			// this chat starts from a real read — with its quiet clock at zero,
			// which is what stops an unreadable screen ever looking quiescent.
			return screenCache{session: runner.TerminalSession}
		}
		// The screen has not moved. This is the steady state for a chat parked on
		// a dialog — and for every idle chat in the daemon — so it must, and does,
		// cost one integer compare. The quiet clock is carried, not restarted:
		// stillness is exactly what it is measuring.
		carried := prev
		carried.gen = gen
		return carried
	}

	// Bytes arrived — which is NOT the same as the screen having changed. A TUI
	// that repaints byte-identical cells bumps the generation without moving
	// anything a user could see, and treating that as movement would reset the
	// quiet clock on a schedule the CLI controls: a repaint period shorter than
	// stallQuiet would defer the close forever and put the wedge back with every
	// test still green.
	//
	// So the freshly rendered text decides. Identical text carries the ENTIRE
	// previous answer forward — the clock, both matches, and the one-shot latch —
	// because none of them is a function of anything that changed. Only the
	// generation advances, so the next tick can go back to comparing integers.
	//
	// A NEW PTY is excluded from this: its content may coincidentally equal the
	// dead process's last screen, and crediting a fresh runner with its
	// predecessor's stillness would close its turn on the first tick.
	if !fresh && prev.text == text && !prev.since.IsZero() {
		carried := prev
		carried.gen = gen
		return carried
	}

	// The screen genuinely moved, so every answer about it is recomputed from
	// scratch — including the quiet clock, which any real change resets to zero.
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

// stalled decides whether this working chat's turn has been abandoned, and
// latches the answer into the screen cache so it is given once.
//
// The in-memory gates come first and the two repository reads come last. That is
// the opposite of the wait path's ordering and for the same reason: there, the
// screen render was the expensive thing and the choices read was cheap relative
// to it; here the screen has already been read for other purposes, so the
// database is the only cost left — and asking it every tick for every working
// chat in the daemon would be two queries per chat per interval, forever, to
// answer a question that matters at most once in a chat's life. All six gates
// must hold either way, and conjunction does not care about order.
//
// It is called only for a chat the caller has already established IS Working.
func (d *detector) stalled(
	ctx context.Context,
	runner domain.AgentRunner,
	screen *screenCache,
) bool {
	if screen.fired {
		return false
	}
	// A question that cannot be asked is never answered "yes". Any of the three
	// stall dependencies missing means this daemon does not close turns here.
	if d.deps.Notices == nil || d.deps.Work == nil || d.deps.OnStall == nil {
		return false
	}
	// No screen, or a screen whose clock has not started, is no evidence.
	if screen.gen == 0 || screen.since.IsZero() {
		return false
	}
	// The corroborating half. A notice that does not declare ends_turn says
	// nothing about whether the CLI is working, and no notice at all says less.
	if !screen.notice.EndsTurn {
		return false
	}
	// The independent half. A screen that has moved at all is a CLI that is
	// doing something, whatever its banner says.
	if d.now().Sub(screen.since) < d.stallQuiet() {
		return false
	}
	// A chat blocked on a prompt is waiting on the human, and its Working is
	// honest. A failed read is the same answer: not sure, so not closing.
	pending, err := d.deps.Choices.PendingChoices(ctx, runner.CurrentChatID)
	if err != nil || len(pending) > 0 {
		return false
	}
	// The gate that does not look at the screen at all. A tool call or subagent
	// still open in the conversation record is HOOK evidence that the CLI is
	// working — the provider announced the start and has not announced the end —
	// and hook evidence outranks a still screen, because the still screen is the
	// half of this that is measured for one provider and assumed for the other.
	//
	// Its own failure mode is a tool whose completion hook never arrives, which
	// leaves this chat permanently ineligible and therefore permanently wedged.
	// That is the direction every decision in this package leans: a chat that
	// stays spinning is a visible bug somebody will report, and a spinner that
	// went dark under a working agent is a silent one that eats the record of
	// what it was doing.
	open, err := d.deps.Work.OpenWork(ctx, runner.CurrentChatID)
	if err != nil || open {
		return false
	}

	screen.fired = true
	return true
}
