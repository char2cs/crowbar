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

type sessionState uint8

const (
	stateLive sessionState = iota + 1
	stateSuspended
	stateExited
	stateRemoved
)

type sessionEntry struct {
	id      string
	chatID  string
	command bool

	mu         sync.Mutex
	state      sessionState
	sess       atomic.Pointer[session.Session]
	shell      string
	cwd        string
	profileID  string
	blob       []byte
	onExit     func(Exit)
	lastActive time.Time
}

type effects []func()

func (fx effects) run() {
	for _, f := range fx {
		f()
	}
}

func (e *terminalEngine) endLocked(ctx context.Context, ent *sessionEntry, to sessionState, exitCode int) effects {
	e.unregister(ent)
	if !ent.command {
		e.discardPersisted(ctx, ent)
	}
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

func (e *terminalEngine) parkLocked(ctx context.Context, ent *sessionEntry, s *session.Session, blob []byte) {
	ent.cwd = s.CWD()
	ent.lastActive = time.Now()
	e.saveMeta(ctx, ent, "suspended")
	ent.state = stateSuspended
	ent.sess.Store(nil)
	ent.blob = blob
	s.Kill()
}

func (e *terminalEngine) killLocked(ctx context.Context, ent *sessionEntry) effects {
	switch ent.state {
	case stateLive:
		s := ent.sess.Load()
		s.Kill()
		return e.endLocked(ctx, ent, stateExited, s.ExitCode())
	case stateSuspended:
		return e.endLocked(ctx, ent, stateRemoved, -1)
	case stateExited, stateRemoved:
		return nil
	}
	return nil
}

func (e *terminalEngine) Kill(ctx context.Context, sessionID string) error {
	ent, ok := e.lookup(sessionID)
	if !ok {
		return fmt.Errorf("terminal: kill: %w: %s", ErrSessionNotFound, sessionID)
	}
	ent.mu.Lock()
	fx := e.killLocked(ctx, ent)
	ent.mu.Unlock()
	fx.run()
	return nil
}

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

func (e *terminalEngine) Shutdown() {
	e.stopOnce.Do(func() { close(e.stop) })
	<-e.maintDone
	reaped := e.reaps.drain()
	ctx := context.Background()
	for _, ent := range e.snapshot() {
		ent.mu.Lock()
		fx := e.shutdownLocked(ctx, ent)
		e.unregister(ent)
		ent.mu.Unlock()
		fx.run()
	}
	<-reaped
}

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

func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
