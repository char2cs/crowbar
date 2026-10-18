// Package runner (file supervisor.go) is the session supervisor's record of
// how each chat's last runner ended. The snapshot joins it onto every chat
// frame, so a client renders the reason and never infers it.
package runner

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// sessionBook is in memory only, like every live-process registry here: a
// daemon restart is itself recorded (AgentExitDaemonRestart) at boot. A nil
// book (a bare Runners built by a test) records nothing.
type sessionBook struct {
	mu    sync.Mutex
	chats map[string]domain.AgentSession
	// causes is the reason noted for a runner BEFORE it ends, read once by
	// the exit that produces. A runner with none exited on its own.
	causes map[string]string
}

// backgroundWork is the supervisor's own follow-up work (a native view's
// exit handling): tracked, so Shutdown cancels it and waits, and none outlives
// the daemon. A nil one (a bare Runners built by a test) runs untracked.
type backgroundWork struct {
	mu      sync.Mutex
	stopped bool
	cancels map[*context.CancelFunc]struct{}
	wg      sync.WaitGroup
}

func newBackgroundWork() *backgroundWork {
	return &backgroundWork{cancels: map[*context.CancelFunc]struct{}{}}
}

// run starts fn with parent's values but the supervisor's lifetime: it
// outlives the request that asked for it and ends with Shutdown.
func (b *backgroundWork) run(parent context.Context, fn func(context.Context)) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	if b == nil {
		go func() {
			defer cancel()
			fn(ctx)
		}()
		return
	}
	b.mu.Lock()
	if b.stopped {
		cancel() // after Shutdown it runs already cancelled, and is still waited for
	}
	b.cancels[&cancel] = struct{}{}
	b.wg.Add(1)
	b.mu.Unlock()
	go func() {
		defer b.wg.Done()
		defer b.forget(&cancel)
		fn(ctx)
	}()
}

func (b *backgroundWork) forget(cancel *context.CancelFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.cancels, cancel)
	(*cancel)()
}

// stop cancels every piece of background work and waits for it to return.
func (b *backgroundWork) stop() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.stopped = true
	for cancel := range b.cancels {
		(*cancel)()
	}
	b.mu.Unlock()
	b.wg.Wait()
}

func newSessionBook() *sessionBook {
	return &sessionBook{
		chats:  map[string]domain.AgentSession{},
		causes: map[string]string{},
	}
}

func (b *sessionBook) get(chatID string) domain.AgentSession {
	if b == nil {
		return domain.AgentSession{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.chats[chatID]
}

func (b *sessionBook) set(chatID string, s domain.AgentSession) {
	if b == nil || chatID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chats[chatID] = s
}

func (b *sessionBook) cause(runnerID, reason string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.causes[runnerID] = reason
}

// takeCause consumes runnerID's noted cause; "exited" when none was noted.
func (b *sessionBook) takeCause(runnerID string) string {
	if b == nil {
		return domain.AgentExitExited
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	reason, noted := b.causes[runnerID]
	delete(b.causes, runnerID)
	if !noted {
		return domain.AgentExitExited
	}
	return reason
}

func (b *sessionBook) forget(chatID string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.chats, chatID)
}

// Session implements snapshot.Runtime.
func (rs *Runners) Session(chatID string) domain.AgentSession {
	return rs.sessions.get(chatID)
}

// noteLaunch clears chatID's exit record: a runner is live on it again.
func (rs *Runners) noteLaunch(ctx context.Context, chatID string) {
	slog.InfoContext(ctx, "agent: runner launched", "component", "runner", "chat", chatID)
	rs.sessions.set(chatID, domain.AgentSession{})
	rs.touch(ctx, chatID)
}

// noteExit records how chatID's runner (runnerID) ended, consuming the cause
// noted for it. A runner already off its chat (displaced) records nothing.
func (rs *Runners) noteExit(ctx context.Context, chatID, runnerID string) {
	reason := rs.sessions.takeCause(runnerID)
	if chatID == "" {
		return
	}
	rs.noteChatExit(ctx, chatID, reason)
}

// noteChatExit records a reason on chatID directly, for an end no single
// runner exit carries (a stop, a failed spawn, a daemon restart).
func (rs *Runners) noteChatExit(ctx context.Context, chatID, reason string) {
	// displaced is an intermediate note: a Stop or switch that follows records
	// the final reason, so only that one is Info.
	level := slog.LevelInfo
	if reason == domain.AgentExitDisplaced {
		level = slog.LevelDebug
	}
	slog.Log(ctx, level, "agent: runner ended", "component", "runner", "chat", chatID, "reason", reason)
	rs.sessions.set(chatID, domain.AgentSession{ExitReason: reason, ExitedAt: time.Now()})
	rs.touch(ctx, chatID)
}

// noteSpawnFailure records spawn_failed on a chat a spawn left with no runner.
// A CLI that died during startup already recorded why through its own exit, and
// a preempted spawn is the preempting Stop's to record.
func (rs *Runners) noteSpawnFailure(ctx context.Context, chatID string, err *error) {
	if *err == nil || errors.Is(*err, ErrProviderExitedDuringStartup) || errors.Is(*err, context.Canceled) {
		return
	}
	if _, liveErr := rs.runnerStore.LiveRunnerForChat(ctx, chatID); liveErr == nil {
		return
	}
	rs.noteChatExit(ctx, chatID, domain.AgentExitSpawnFailed)
}
