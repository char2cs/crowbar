package agent

import "sync"

// waitingForTurnLog is logged the instant a provider switch parks on an in-flight turn.
// It is the only visible sign of a switch that is taking a while, and the user is looking
// at a spinner while it happens — so it is an INFO line, not a debug one.
const waitingForTurnLog = "agent: switch provider: the chat is mid-turn; waiting for the CLI to finish before quitting it"

// turnWaits is the registry of TURNS CURRENTLY IN FLIGHT — one entry per runner that has
// been handed a prompt and has not yet answered it — and the only thing in this package a
// caller may BLOCK on. Its single consumer is the provider switch: a switch must never
// decapitate a turn, because a vendor CLI killed mid-answer never flushes its native
// transcript, which loses the answer AND leaves nothing for the next `--resume` to find
// ("No conversation found with session ID", hit in production).
//
// IT IS KEYED BY RUNNER, NOT BY CHAT, and that is not a detail. An in-flight turn is a
// PROCESS's turn: it ends when the process says so (turn_stop), when the process leaves
// the chat (a /clear or /resume inside the CLI), when Crowbar takes it off the chat
// (displace), or when it dies. Keying by runner is what lets every one of those four
// releases be expressed exactly — and, in particular, is what stops a belated exit of the
// CLI a switch just quit from releasing a waiter parked on the turn of the CLI that
// REPLACED it.
//
// It holds no liveness opinion. It never says a process is alive, only that Crowbar has
// seen a prompt go in and no answer come out — and it is consulted for exactly one
// decision: whether to wait before killing. Its entries are as process-scoped as the PTYs
// they follow (a daemon restart empties it, and a restart kills every CLI it could have
// been describing), so it can never survive to lie.
//
// WHY NOT domain.AgentChat.Working, which says the same thing? Because Working is a READ
// MODEL, and the turn commands are deliberately on asynx's ASYNC Send path (blocking a
// hook on a projection is the contention shape behind this repo's history of git-mutex
// hangs, see agentchat.EventStore.Create's doc). So Working lags the hooks in BOTH
// directions: it can still read true for a turn that has already stopped — which would
// park a switch forever on a turn nobody is running — and false for one that has just
// started, which is the very bug this fixes, merely in a narrower window. This registry is
// fed from the same hooks Working is fed from, at the moment the command is issued, with
// no projection in between.
type turnWaits struct {
	mu      sync.Mutex
	turns   map[string]*inflightTurn
	changed map[string]chan struct{}
}

// inflightTurn is one runner's open turn: the chat it is answering into, and the channel
// that is CLOSED when it stops being in flight. A closed channel (rather than a value) is
// the release, so any number of waiters wake and a waiter that arrives late never blocks.
type inflightTurn struct {
	chatID string
	done   chan struct{}
}

func newTurnWaits() *turnWaits {
	return &turnWaits{
		turns:   map[string]*inflightTurn{},
		changed: map[string]chan struct{}{},
	}
}

func (w *turnWaits) changedLocked(chatID string) chan struct{} {
	ch := w.changed[chatID]
	if ch == nil {
		ch = make(chan struct{})
		w.changed[chatID] = ch
	}
	return ch
}

func (w *turnWaits) signalLocked(chatID string) {
	if chatID == "" {
		return
	}
	close(w.changedLocked(chatID))
	w.changed[chatID] = make(chan struct{})
}

// begin records that runnerID is now working on a turn in chatID.
//
// A second prompt while a turn is already open on the SAME chat is not a new turn — the
// CLI is still working, and its one turn_stop closes the lot. Re-opening it would hand a
// waiter a fresh channel and orphan the one it is already blocked on.
//
// A turn open on a DIFFERENT chat means the runner has moved without us being told, so the
// old turn can never be closed where it stands: release it.
func (w *turnWaits) begin(
	runnerID string,
	chatID string,
) {
	if runnerID == "" || chatID == "" {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if prev, ok := w.turns[runnerID]; ok {
		if prev.chatID == chatID {
			return
		}
		close(prev.done)
		w.signalLocked(prev.chatID)
	}
	w.turns[runnerID] = &inflightTurn{chatID: chatID, done: make(chan struct{})}
	w.signalLocked(chatID)
}

// complete ends runnerID's turn and releases everyone waiting on it. It is called on
// every way a turn can stop being in flight — the CLI answered, the CLI left the chat,
// Crowbar took it off the chat, the process died — because a waiter released only by the
// FIRST of those would hang on all the others. Completing a runner with no open turn is a
// no-op, so every one of those paths can call it unconditionally.
func (w *turnWaits) complete(
	runnerID string,
) {
	if runnerID == "" {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	t, ok := w.turns[runnerID]
	if !ok {
		return
	}
	delete(w.turns, runnerID)
	close(t.done)
	w.signalLocked(t.chatID)
}

// inflight snapshots the release channel of every turn currently open on chatID. Empty —
// the overwhelmingly common case — means the chat is idle and a caller need not wait for
// anything at all.
//
// It returns EVERY open turn, not "the" one. I2 (at most one live runner per chat) makes
// that a set of size one in a settled model, and the switch that calls this holds the
// chat's spawn gate so no NEW runner can arrive on it — but the placement paths that heal
// a broken invariant run off the hook path, which is not gated, and a waiter that watched
// only the first turn it found could sail past the other.
func (w *turnWaits) inflight(
	chatID string,
) []<-chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()

	var open []<-chan struct{}
	for _, t := range w.turns {
		if t.chatID == chatID {
			open = append(open, t.done)
		}
	}
	return open
}

// watch returns whether chatID has any open turn and a channel closed on the next
// begin/complete transition for that chat. Capturing the state and signal under one
// mutex prevents a switch from missing the transition that should wake it.
func (w *turnWaits) watch(chatID string) (bool, <-chan struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()

	open := false
	for _, t := range w.turns {
		if t.chatID == chatID {
			open = true
			break
		}
	}
	return open, w.changedLocked(chatID)
}

// chatWorkStates is the process-local mirror of the authoritative aggregate
// returned by StartTurn/StopTurn/AbandonTurn. It exists because GetChat is an
// asynchronous read model: a destructive switch cannot use a projection lag as
// permission to kill a CLI whose background work is still live.
//
// Unknown means this process has not issued a turn command for the chat. Callers
// may seed that one read from the durable projection (settled chats and pre-existing
// chats after boot); once known, only command results may change it.
type chatWorkStates struct {
	mu     sync.Mutex
	states map[string]*chatWorkState
}

type chatWorkState struct {
	known   bool
	working bool
	changed chan struct{}
}

func newChatWorkStates() *chatWorkStates {
	return &chatWorkStates{states: map[string]*chatWorkState{}}
}

func (w *chatWorkStates) stateLocked(chatID string) *chatWorkState {
	state := w.states[chatID]
	if state == nil {
		state = &chatWorkState{changed: make(chan struct{})}
		w.states[chatID] = state
	}
	return state
}

// set records the aggregate returned by a synchronous command dispatch and wakes
// every switch waiting for the chat to become idle. It signals even when Working
// did not change: a Stop can restate a lower async-work level while remaining true,
// and the next restatement is still semantically newer.
func (w *chatWorkStates) set(chatID string, working bool) {
	if chatID == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	state := w.stateLocked(chatID)
	state.known = true
	state.working = working
	close(state.changed)
	state.changed = make(chan struct{})
}

// observe atomically snapshots the latest authoritative state and the signal for
// the next change. A closed signal can never be paired with stale state.
func (w *chatWorkStates) observe(chatID string) (working, known bool, changed <-chan struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()

	state := w.stateLocked(chatID)
	return state.working, state.known, state.changed
}
