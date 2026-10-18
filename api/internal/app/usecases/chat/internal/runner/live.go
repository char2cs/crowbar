// Package runner (file live.go) is the supervisor's table of live CLIs: one
// entry per chat, keyed so a second can never be added. It is in memory only;
// what survives a restart is the ledger's runner_started/runner_exited rows.
package runner

import (
	"context"
	"sync"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type liveTable struct {
	mu      sync.Mutex
	chats   map[string]*liveEntry
	runners map[string]*liveEntry
	// turnStarts orders a turn opening on a chat's runner against its end: an
	// end marked under it closes every turn opened before, and none opens after.
	// Only inTurnOrder takes it.
	turnStarts *inflight.Gate
}

type liveEntry struct {
	runner engineagents.Runner
	// started is set once its runner_started row has committed; until then
	// the runner is a spawn in progress, which no reader outside it is shown.
	started bool
	// exiting is set once the runner's end is asked for or under way; its
	// hooks still resolve until remove.
	exiting bool
	// claimed is set by the one exit that records the runner's end.
	claimed bool
	exited  chan struct{} // closed by remove
	cause   string        // the first exit reason noted; "" = exited on its own
	code    int           // the process's exit code, once it has one
}

// liveView is one read of a chat's entry.
type liveView struct {
	runner                    engineagents.Runner
	started, exiting, claimed bool
	exited                    <-chan struct{}
}

func newLiveTable() *liveTable {
	return &liveTable{
		chats: map[string]*liveEntry{}, runners: map[string]*liveEntry{}, turnStarts: inflight.NewGate(),
	}
}

// add registers r on r.CurrentChatID, refusing while that chat has an entry.
func (t *liveTable) add(r engineagents.Runner) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, taken := t.chats[r.CurrentChatID]; taken {
		return ErrChatHasLiveRunner
	}
	e := &liveEntry{runner: r, exited: make(chan struct{})}
	t.chats[r.CurrentChatID], t.runners[r.ID] = e, e
	return nil
}

func (t *liveTable) view(chatID string) (liveView, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.chats[chatID]
	if !ok {
		return liveView{}, false
	}
	return liveView{runner: e.runner, started: e.started, exiting: e.exiting, claimed: e.claimed, exited: e.exited}, true
}

// byChat is the runner on chatID, exiting or not.
func (t *liveTable) byChat(chatID string) (engineagents.Runner, bool) {
	v, ok := t.view(chatID)
	return v.runner, ok
}

// byRunner is runnerID's runner, exiting or not.
func (t *liveTable) byRunner(runnerID string) (engineagents.Runner, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.runners[runnerID]; ok {
		return e.runner, true
	}
	return engineagents.Runner{}, false
}

// inTurnOrder runs fn under chatID's turn-start gate: a turn opens on the
// chat's runner (TurnRunner), or that runner's end is marked, inside one.
func (t *liveTable) inTurnOrder(chatID string, fn func()) {
	defer t.turnStarts.Lock(chatID)()
	fn()
}

// unending is runnerID's runner while its end is not under way.
func (t *liveTable) unending(runnerID string) (engineagents.Runner, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.runners[runnerID]; ok && !e.exiting {
		return e.runner, true
	}
	return engineagents.Runner{}, false
}

// update applies change to runnerID's runner while its exit is unclaimed.
func (t *liveTable) update(runnerID string, change func(*engineagents.Runner)) (engineagents.Runner, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.runners[runnerID]
	if !ok || e.claimed {
		return engineagents.Runner{}, false
	}
	change(&e.runner)
	return e.runner, true
}

// markStarted records that runnerID's runner_started row has committed.
func (t *liveTable) markStarted(runnerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.runners[runnerID]; ok {
		e.started = true
	}
}

// markExiting notes cause and marks runnerID's end as under way, and returns
// the channel its exit closes.
func (t *liveTable) markExiting(runnerID, cause string) (<-chan struct{}, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.runners[runnerID]
	if !ok {
		return nil, false
	}
	e.noteCause(cause)
	e.exiting = true
	return e.exited, true
}

// noteCause records why runnerID will end, if it ends, without saying that it
// is ending: whoever waits on it still sees it live until its end is asked for.
func (t *liveTable) noteCause(runnerID, cause string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.runners[runnerID]; ok {
		e.noteCause(cause)
	}
}

// noteCause keeps the first cause noted.
func (e *liveEntry) noteCause(cause string) {
	if e.cause == "" {
		e.cause = cause
	}
}

// keepLive takes back an end that never got under way: the process is still
// running, so its runner holds the chat as before.
func (t *liveTable) keepLive(runnerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.runners[runnerID]; ok && !e.claimed {
		e.exiting, e.cause = false, ""
	}
}

// noteExitCode keeps the code of a process whose exit is recorded later.
func (t *liveTable) noteExitCode(runnerID string, code int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.runners[runnerID]; ok {
		e.code = code
	}
}

// claimExit hands runnerID's entry to the one exit that records it; every
// later call is refused.
func (t *liveTable) claimExit(runnerID string) (liveEntry, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.runners[runnerID]
	if !ok || e.claimed {
		return liveEntry{}, false
	}
	e.claimed, e.exiting = true, true
	return liveEntry{runner: e.runner, started: e.started, cause: e.cause, code: e.code}, true
}

// remove drops runnerID and closes its exit channel: the chat is free.
func (t *liveTable) remove(runnerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.runners[runnerID]
	if !ok {
		return
	}
	delete(t.runners, runnerID)
	delete(t.chats, e.runner.CurrentChatID)
	close(e.exited)
}

// exitAll notes cause on every runner in the table.
func (t *liveTable) exitAll(cause string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, e := range t.runners {
		e.noteCause(cause)
		e.exiting = true
	}
}

// all is every started runner, exiting or not.
func (t *liveTable) all() []engineagents.Runner {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]engineagents.Runner, 0, len(t.runners))
	for _, e := range t.runners {
		if e.started {
			out = append(out, e.runner)
		}
	}
	return out
}

// LiveRunnerForChat is the runner on chatID, ErrNoLiveRunner when it is
// dormant. A runner whose end is under way still answers until it exits.
func (rs *Runners) LiveRunnerForChat(_ context.Context, chatID string) (engineagents.Runner, error) {
	if r, ok := rs.live.byChat(chatID); ok {
		return r, nil
	}
	return engineagents.Runner{}, ErrNoLiveRunner
}

// AllLive is every started runner in the table.
func (rs *Runners) AllLive(context.Context) ([]engineagents.Runner, error) {
	return rs.live.all(), nil
}

// RunnerByID is runnerID's runner while it is in the table.
func (rs *Runners) RunnerByID(runnerID string) (engineagents.Runner, bool) {
	return rs.live.byRunner(runnerID)
}

// TurnRunner runs open on runnerID's runner held against its end, and reports
// false without running it once that end is under way: no turn may open then.
func (rs *Runners) TurnRunner(runnerID string, open func(engineagents.Runner) error) (bool, error) {
	r, ok := rs.live.byRunner(runnerID)
	if !ok {
		return false, nil
	}
	var err error
	rs.live.inTurnOrder(r.CurrentChatID, func() {
		if r, ok = rs.live.unending(runnerID); ok {
			err = open(r)
		}
	})
	return ok, err
}

// LiveRunner implements snapshot.Runtime: a runner is on the chat from its
// runner_started row until its exit is being recorded.
func (rs *Runners) LiveRunner(chatID string) (engineagents.Runner, bool) {
	v, ok := rs.live.view(chatID)
	if !ok || !v.started || v.claimed {
		return engineagents.Runner{}, false
	}
	return v.runner, true
}

// awaitDormant waits out a runner on chatID whose end is under way, and
// returns the runner live once it has, if any.
func (rs *Runners) awaitDormant(ctx context.Context, chatID string) (engineagents.Runner, bool, error) {
	for {
		v, ok := rs.live.view(chatID)
		if !ok {
			return engineagents.Runner{}, false, nil
		}
		if !v.exiting {
			return v.runner, true, nil
		}
		select {
		case <-v.exited:
		case <-ctx.Done():
			return engineagents.Runner{}, false, ctx.Err()
		}
	}
}
