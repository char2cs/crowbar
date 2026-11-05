package terminal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

// sessionState changes only via transition methods under the entry lock: create→Live,
// Live⇄Suspended, Live→Exited, Suspended→Removed. Only Suspended is restorable; the terminal
// states hand out the ended event exactly once, since the locked state check admits one.
type sessionState uint8

const (
	stateLive sessionState = iota + 1
	stateSuspended
	stateExited
	stateRemoved
)

// sessionEntry owns one session's lifecycle and is never recycled, so a waiter keeps the same
// mutex as the operation it raced. Lock order: entry.mu → session locks → engine.regMu.
type sessionEntry struct {
	id      string
	chatID  string
	command bool // an agentic vendor CLI: never suspended, never persisted

	mu    sync.Mutex
	state sessionState
	// sess is non-nil exactly while Live; written only under mu but read without it, so keystroke
	// paths never queue behind lifecycle disk I/O. A just-died session's calls simply fail.
	sess atomic.Pointer[session.Session]
	// shell/cwd/profileID are what a restore re-spawns. cwd is refreshed from the live
	// session's OSC 7 reports whenever it is suspended.
	shell     string
	cwd       string
	profileID string
	// blob is the serialized screen a restore rebuilds from; set only while Suspended.
	blob []byte
	// onExit is told how a command session ended, once, at Exited.
	onExit     func(Exit)
	lastActive time.Time
}

// effects are the callbacks a transition owes its caller (a command session's onExit).
// They are collected under the entry lock and run after it is released, so no callback
// can ever re-enter the engine while it holds a lifecycle lock.
type effects []func()

func (fx effects) run() {
	for _, f := range fx {
		f()
	}
}

// ── transitions ──────────────────────────────────────────────────────────────

// endLocked moves ent to Exited or Removed and returns the callbacks owed; it runs at most once
// since every caller checks the state under ent.mu. Caller holds ent.mu.
func (e *terminalEngine) endLocked(ctx context.Context, ent *sessionEntry, to sessionState, exitCode int) effects {
	e.unregister(ent)
	e.discardPersisted(ctx, ent)
	ent.state = to
	var fx effects
	if onExit := ent.onExit; onExit != nil {
		exit := Exit{Code: exitCode}
		if s := ent.sess.Load(); s != nil {
			exit.Screen, _, _ = s.ScreenText(0)
		}
		ent.onExit = nil
		fx = append(fx, func() { onExit(exit) })
	}
	ent.sess.Store(nil)
	ent.blob = nil
	return fx
}

// restoreLocked respawns a Suspended entry from its screen (home dir if the cwd is gone). An
// unrespawnable one is Removed; one refused for shutdown stays Suspended. Caller holds ent.mu.
func (e *terminalEngine) restoreLocked(ctx context.Context, ent *sessionEntry) (effects, error) {
	if err := e.admit(); err != nil {
		return nil, fmt.Errorf("terminal: restore: %w", err)
	}
	cwd, notice := resolveRestoreCWD(ent.cwd)
	s, err := e.spawnShell(ctx, ent.id, ent.shell, cwd, ent.profileID, engineBirth{Blob: ent.blob, Notice: notice})
	if err != nil {
		e.reaps.settle(false)
		return e.endLocked(ctx, ent, stateRemoved, -1), fmt.Errorf("terminal: restore: spawn: %w", err)
	}
	ent.state = stateLive
	ent.sess.Store(s)
	ent.cwd = cwd
	ent.blob = nil
	e.startReaper(ctx, ent, s)
	e.reaps.settle(true)
	e.saveMeta(ctx, ent, "detached")
	return nil, nil
}

// suspendLocked persists the screen (.buf, then meta row) before killing the process; if that
// fails the session stays Live, since a suspended session with no .buf would restore blank.
func (e *terminalEngine) suspendLocked(ctx context.Context, ent *sessionEntry, force bool) {
	if ent.state != stateLive || ent.command {
		return
	}
	s := ent.sess.Load()
	if !s.SuspendEligible(force) {
		return
	}
	var blob []byte
	if force && !s.IsIdle() {
		// Force-suspend of a LIVE app: teardown + notice + serialize in one session-lock
		// hold so no live chunk can re-alt the model between them.
		blob = s.ForceSuspendSnapshot(
			[]byte("\r\n[crowbar] session suspended to free resources; re-open to restore\r\n"),
		)
	} else {
		blob, _ = s.Snapshot()
	}
	if err := e.writeBuf(ctx, ent, blob); err != nil {
		slog.Warn("terminal: suspend: keeping session live, screen not persisted", "session", ent.id, "err", err)
		return
	}
	e.parkLocked(ctx, ent, s, blob)
}

// parkLocked completes a Live → Suspended transition whose screen blob has already been
// captured: it records the "suspended" meta row, moves the entry to Suspended holding
// blob, and kills the process. Caller holds ent.mu and has checked ent is Live with s.
func (e *terminalEngine) parkLocked(ctx context.Context, ent *sessionEntry, s *session.Session, blob []byte) {
	ent.cwd = s.CWD()
	ent.lastActive = time.Now()
	e.saveMeta(ctx, ent, "suspended")
	ent.state = stateSuspended
	ent.sess.Store(nil)
	ent.blob = blob
	// The reaper wakes on this death, sees the entry has moved on, and does nothing.
	s.Kill()
}

// killLocked ends ent immediately: a Live process group is SIGKILLed and reaped, a
// Suspended session is dropped. Caller holds ent.mu.
func (e *terminalEngine) killLocked(ctx context.Context, ent *sessionEntry) effects {
	switch ent.state {
	case stateLive:
		s := ent.sess.Load()
		s.Kill() // returns once the process is reaped, so the exit code is final
		return e.endLocked(ctx, ent, stateExited, s.ExitCode())
	case stateSuspended:
		return e.endLocked(ctx, ent, stateRemoved, -1)
	case stateExited, stateRemoved:
		return nil
	}
	return nil
}

// ── operations ───────────────────────────────────────────────────────────────

func (e *terminalEngine) Kill(ctx context.Context, sessionID string) error {
	ent, ok := e.lookup(sessionID)
	if !ok {
		return fmt.Errorf("terminal: kill: %w: %s", ErrSessionNotFound, sessionID)
	}
	// An entry that ended between the lookup and the lock (its process exited on its own)
	// has already reached the state this Kill asked for.
	ent.mu.Lock()
	fx := e.killLocked(ctx, ent)
	ent.mu.Unlock()
	fx.run()
	return nil
}

// TerminateGraceful SIGTERMs the process group, waits up to grace OUTSIDE the lifecycle lock,
// then hard-kills, and ends whatever the session became exactly like Kill.
func (e *terminalEngine) TerminateGraceful(ctx context.Context, sessionID string) error {
	ent, ok := e.lookup(sessionID)
	if !ok {
		return fmt.Errorf("terminal: terminate: %w: %s", ErrSessionNotFound, sessionID)
	}
	if s := ent.live(); s != nil {
		s.Terminate(e.cfg.terminateGrace)
	}
	ent.mu.Lock()
	fx := e.killLocked(ctx, ent)
	ent.mu.Unlock()
	fx.run()
	return nil
}

// Suspend tears down an idle, unattached shell's PTY, persisting its screen.
func (e *terminalEngine) Suspend(ctx context.Context, sid string) error {
	e.suspend(ctx, sid, false)
	return nil
}

func (e *terminalEngine) suspend(ctx context.Context, sid string, force bool) {
	ent, ok := e.lookup(sid)
	if !ok {
		return
	}
	ent.mu.Lock()
	e.suspendLocked(ctx, ent, force)
	ent.mu.Unlock()
}

// Shutdown persists live shells as Suspended (no exit frame: they did not exit), kills command
// sessions so their onExit runs, stops further births, and blocks until every reaper returned.
// Idempotent.
func (e *terminalEngine) Shutdown() {
	e.stopOnce.Do(func() { close(e.stop) })
	// JOIN the maintenance goroutine before tearing down sessions, so no in-flight sweep
	// runs concurrently with the teardown.
	<-e.maintDone
	// Close the door on new sessions BEFORE walking the registry: a session born mid-walk
	// would otherwise be missed and left with no reaper. Every live session's reaper is
	// already counted; the kill below is simply what releases it.
	reaped := e.reaps.drain()
	ctx := context.Background()
	for _, ent := range e.snapshot() {
		ent.mu.Lock()
		fx := e.shutdownLocked(ctx, ent)
		// Unload: the persisted rows are the next boot's to restore.
		e.unregister(ent)
		ent.mu.Unlock()
		fx.run()
	}
	// JOIN every reaper, so the callbacks of any session that exited on its own while we
	// were walking have RUN too. Each reaper's first act is to wait on a Done channel the
	// loop above has already closed, so this converges on the work itself.
	<-reaped
}

// shutdownLocked ends ent's process for Shutdown: a live command session is killed and
// Exited; a live shell is persisted and Suspended, even when its screen cannot be written
// (the meta row still lets the next boot restore it). Caller holds ent.mu.
func (e *terminalEngine) shutdownLocked(ctx context.Context, ent *sessionEntry) effects {
	switch ent.state {
	case stateLive:
		s := ent.sess.Load()
		if ent.command {
			s.Kill()
			return e.endLocked(ctx, ent, stateExited, s.ExitCode())
		}
		blob, _ := s.Snapshot()
		if err := e.writeBuf(ctx, ent, blob); err != nil {
			slog.Warn("terminal: shutdown: screen not persisted", "session", ent.id, "err", err)
		}
		e.parkLocked(ctx, ent, s, blob)
		return nil
	case stateSuspended, stateExited, stateRemoved:
		return nil
	}
	return nil
}

// resolveRestoreCWD returns a working directory that exists for a restore spawn: the saved
// cwd, else the home directory with a one-line notice saying why the cwd was not used.
func resolveRestoreCWD(cwd string) (string, []byte) {
	if dirExists(cwd) {
		return cwd, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return home, fmt.Appendf(nil, "\r\n[crowbar] previous directory unavailable; restored in %s\r\n", home)
}

// dirExists reports whether path resolves to an existing directory.
func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
