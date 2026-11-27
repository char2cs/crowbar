package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type liveTable struct {
	mu        sync.Mutex
	chats     map[string]*liveEntry
	runners   map[string]*liveEntry
	launching sync.Map
}

type liveEntry struct {
	runner      engineagents.Runner
	turnOrder   *sync.Mutex
	started     bool
	exitedEarly bool
	exiting     bool
	claimed     bool
	exited      chan struct{}
	cause       string
	exit        runnerExit
}

type liveView struct {
	runner                    engineagents.Runner
	started, exiting, claimed bool
	exited                    <-chan struct{}
}

func newLiveTable() *liveTable {
	return &liveTable{chats: map[string]*liveEntry{}, runners: map[string]*liveEntry{}}
}

func (t *liveTable) add(r engineagents.Runner) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, taken := t.chats[r.CurrentChatID]; taken {
		return ErrChatHasLiveRunner
	}
	e := &liveEntry{runner: r, turnOrder: &sync.Mutex{}, exited: make(chan struct{})}
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

func (t *liveTable) byChat(chatID string) (engineagents.Runner, bool) {
	v, ok := t.view(chatID)
	return v.runner, ok
}

func (t *liveTable) byRunner(runnerID string) (engineagents.Runner, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.runners[runnerID]; ok {
		return e.runner, true
	}
	return engineagents.Runner{}, false
}

func (t *liveTable) inTurnOrder(chatID string, fn func()) {
	t.mu.Lock()
	e, ok := t.chats[chatID]
	t.mu.Unlock()
	if ok {
		e.turnOrder.Lock()
		defer e.turnOrder.Unlock()
	}
	fn()
}

func (t *liveTable) unending(runnerID string) (engineagents.Runner, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.runners[runnerID]; ok && !e.exiting {
		return e.runner, true
	}
	return engineagents.Runner{}, false
}

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

func (t *liveTable) markStarted(runnerID string) (exitedEarly bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.runners[runnerID]
	if !ok {
		return false
	}
	e.started = true
	return e.exitedEarly
}

func (t *liveTable) exitBeforeStart(runnerID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.runners[runnerID]
	if !ok || e.started {
		return false
	}
	e.exitedEarly = true
	return true
}

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

func (t *liveTable) noteCause(runnerID, cause string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.runners[runnerID]; ok {
		e.noteCause(cause)
	}
}

func (e *liveEntry) noteCause(cause string) {
	if e.cause == "" {
		e.cause = cause
	}
}

func (t *liveTable) keepLive(runnerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.runners[runnerID]; ok && !e.claimed {
		e.exiting, e.cause = false, ""
	}
}

func (t *liveTable) noteExit(runnerID string, exit runnerExit) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.runners[runnerID]; ok {
		e.exit = exit
	}
}

func (t *liveTable) claimExit(runnerID string) (liveEntry, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.runners[runnerID]
	if !ok || e.claimed {
		return liveEntry{}, false
	}
	e.claimed, e.exiting = true, true
	return liveEntry{runner: e.runner, started: e.started, cause: e.cause, exit: e.exit}, true
}

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

func (t *liveTable) exitAll(cause string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, e := range t.runners {
		e.noteCause(cause)
		e.exiting = true
	}
}

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

func (rs *Runners) LiveRunnerForChat(_ context.Context, chatID string) (engineagents.Runner, error) {
	if r, ok := rs.live.byChat(chatID); ok {
		return r, nil
	}
	return engineagents.Runner{}, ErrNoLiveRunner
}

func (rs *Runners) AllLive(context.Context) ([]engineagents.Runner, error) {
	return rs.live.all(), nil
}

func (rs *Runners) RunnerByID(runnerID string) (engineagents.Runner, bool) {
	return rs.live.byRunner(runnerID)
}

func (rs *Runners) RecordedRunner(ctx context.Context, runnerID string) (engineagents.Runner, bool, error) {
	chatID, started, ok, err := rs.ledger.RunnerStart(ctx, runnerID)
	if err != nil || !ok {
		return engineagents.Runner{}, false, err
	}
	chat, err := rs.chats.GetChat(ctx, chatID)
	if errors.Is(err, agentchat.ErrNotFound) {
		return engineagents.Runner{}, false, nil
	}
	if err != nil {
		return engineagents.Runner{}, false, fmt.Errorf("agent: recorded runner %s: %w", runnerID, err)
	}
	return engineagents.Runner{
		ID: runnerID, ProviderID: started.Provider, WorkspaceID: chat.WorkspaceID, CurrentChatID: chatID,
		TerminalSession: started.TerminalSession,
	}, true, nil
}

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

func (rs *Runners) LiveRunner(chatID string) (engineagents.Runner, bool) {
	v, ok := rs.live.view(chatID)
	if !ok || !v.started || v.claimed {
		return engineagents.Runner{}, false
	}
	return v.runner, true
}

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
