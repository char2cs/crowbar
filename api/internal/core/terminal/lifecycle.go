package terminal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/profile"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// sessionState is where a session is in its life. Every session has exactly one, held in
// its sessionEntry, and every change of it goes through a transition method below while
// the entry's lock is held.
//
//	         create                 suspend
//	(birth) ───────▶ Live ◀──────────────────▶ Suspended
//	                  │         restore           │
//	      exit / kill │                           │ kill / evict / unrestorable
//	                  ▼                           ▼
//	               Exited                      Removed
//
// Only Suspended is restorable. That single rule is what makes resurrection impossible: a
// session whose process died is still Live until its reaper moves it to Exited, and an
// Attach that lands in that window finds a dead process, not a placeholder — it reports
// the exit instead of spawning a stranger under the old id.
//
// Exited and Removed are terminal: the entry has left the registry, its .buf and meta row
// are deleted, and its ended event (plus a command session's onExit) has been handed out
// exactly once — by whichever transition got there, since the state check under the lock
// admits only one.
type sessionState uint8

const (
	stateLive sessionState = iota + 1
	stateSuspended
	stateExited
	stateRemoved
)

// sessionEntry is the single owner of one session's lifecycle. It is created at birth (or
// when a persisted session is loaded) and never recycled: an operation that looked it up
// keeps a pointer that stays valid — and keeps the SAME mutex — after the entry leaves the
// registry, so a waiter can never end up holding a different lock than the operation it
// was racing (the flaw of the old per-id mutex map, whose entries were deleted under it).
//
// Lock order: entry.mu → session locks → engine.regMu.
type sessionEntry struct {
	id      string
	chatID  string
	command bool // an agentic vendor CLI: never suspended, never persisted

	mu    sync.Mutex
	state sessionState
	// sess is the running process; non-nil exactly while Live. It is written only under
	// mu (by a transition) but may be READ without it: the per-keystroke paths (Write,
	// Resize, Screen) must never queue behind a lifecycle operation doing disk I/O. A
	// reader may see a session that has just died — its calls then fail, which is the
	// same answer they would get a moment later.
	sess atomic.Pointer[session.Session]
	// shell/cwd/profileID are what a restore re-spawns. cwd is refreshed from the live
	// session's OSC 7 reports whenever it is suspended.
	shell     string
	cwd       string
	profileID string
	// blob is the serialized screen a restore rebuilds from; set only while Suspended.
	blob []byte
	// onExit is a CreateCommand caller's cleanup, handed out once at Exited.
	onExit     func()
	lastActive time.Time
}

// effects are the callbacks a transition owes the outside world. They are collected under
// the entry lock and run after it is released, so no callback can ever re-enter the engine
// while it holds a lifecycle lock.
type effects []func()

func (fx effects) run() {
	for _, f := range fx {
		f()
	}
}

// ── registry ─────────────────────────────────────────────────────────────────

func (e *terminalEngine) lookup(id string) (*sessionEntry, bool) {
	e.regMu.RLock()
	defer e.regMu.RUnlock()
	ent, ok := e.entries[id]
	return ent, ok
}

func (e *terminalEngine) register(ent *sessionEntry) {
	e.regMu.Lock()
	defer e.regMu.Unlock()
	e.entries[ent.id] = ent
	ids := e.byChat[ent.chatID]
	if ids == nil {
		ids = make(map[string]*sessionEntry)
		e.byChat[ent.chatID] = ids
	}
	ids[ent.id] = ent
}

// unregister drops ent from the registry. Idempotent, and a no-op if the id now names a
// different entry.
func (e *terminalEngine) unregister(ent *sessionEntry) {
	e.regMu.Lock()
	defer e.regMu.Unlock()
	if e.entries[ent.id] != ent {
		return
	}
	delete(e.entries, ent.id)
	ids := e.byChat[ent.chatID]
	delete(ids, ent.id)
	if len(ids) == 0 {
		delete(e.byChat, ent.chatID)
	}
}

// snapshot returns every registered entry.
func (e *terminalEngine) snapshot() []*sessionEntry {
	e.regMu.RLock()
	defer e.regMu.RUnlock()
	out := make([]*sessionEntry, 0, len(e.entries))
	for _, ent := range e.entries {
		out = append(out, ent)
	}
	return out
}

func (e *terminalEngine) ListSessions() []string {
	e.regMu.RLock()
	defer e.regMu.RUnlock()
	ids := make([]string, 0, len(e.entries))
	for id := range e.entries {
		ids = append(ids, id)
	}
	return ids
}

// ListSessionsForChat returns the registered session IDs owned by chatID.
func (e *terminalEngine) ListSessionsForChat(chatID string) []string {
	e.regMu.RLock()
	defer e.regMu.RUnlock()
	ids := e.byChat[chatID]
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	return out
}

// ── reads ────────────────────────────────────────────────────────────────────

// live returns the entry's running session, or nil when it is not Live.
func (ent *sessionEntry) live() *session.Session {
	return ent.sess.Load()
}

// liveSession looks id up and returns its running session.
func (e *terminalEngine) liveSession(id string) *session.Session {
	ent, ok := e.lookup(id)
	if !ok {
		return nil
	}
	return ent.live()
}

// stateString is the wire form of the entry's state: "active"/"detached" for a live
// session (by whether a client is attached), "suspended", or "" once it has ended.
// Caller holds ent.mu.
func (ent *sessionEntry) stateStringLocked() string {
	switch ent.state {
	case stateLive:
		if ent.sess.Load().AttachedCount() > 0 {
			return "active"
		}
		return "detached"
	case stateSuspended:
		return "suspended"
	case stateExited, stateRemoved:
		return ""
	}
	return ""
}

// StateOf returns the session's state string ("active", "detached", or "suspended") and
// true, or ("", false) when the session is not registered.
func (e *terminalEngine) StateOf(sessionID string) (string, bool) {
	ent, ok := e.lookup(sessionID)
	if !ok {
		return "", false
	}
	ent.mu.Lock()
	defer ent.mu.Unlock()
	st := ent.stateStringLocked()
	return st, st != ""
}

// SessionExists reports whether the session is registered (live or suspended).
func (e *terminalEngine) SessionExists(_ context.Context, sessionID string) bool {
	_, ok := e.lookup(sessionID)
	return ok
}

// SessionLive reports whether the session id is backed by a running process right now: a
// suspended session, and a live one whose process has exited but not yet been reaped, both
// read false.
func (e *terminalEngine) SessionLive(_ context.Context, sessionID string) bool {
	s := e.liveSession(sessionID)
	if s == nil {
		return false
	}
	select {
	case <-s.Done():
		return false
	default:
		return true
	}
}

// Screen implements the Engine's pull-style screen read. An unknown or suspended session
// reports ("", 0, false).
func (e *terminalEngine) Screen(sessionID string, since uint64) (text string, gen uint64, changed bool) {
	s := e.liveSession(sessionID)
	if s == nil {
		return "", 0, false
	}
	return s.ScreenText(since)
}

func (e *terminalEngine) Write(_ context.Context, sessionID string, data []byte) error {
	s := e.liveSession(sessionID)
	if s == nil {
		return fmt.Errorf("terminal: write: %w: %s", ErrSessionNotFound, sessionID)
	}
	return s.Write(data)
}

func (e *terminalEngine) Resize(_ context.Context, sessionID string, cols, rows uint16) error {
	s := e.liveSession(sessionID)
	if s == nil {
		return fmt.Errorf("terminal: resize: %w: %s", ErrSessionNotFound, sessionID)
	}
	return s.Resize(cols, rows)
}

// ── birth ────────────────────────────────────────────────────────────────────

// engineBirth carries one of the two shell birth modes: create (Blob == nil, size and
// scrollback from Cols/Rows/ScrollbackLines, zero meaning the defaults) or restore (Blob is
// the persisted screen, including its CRWB1 header, plus an optional daemon-authored
// Notice injected into the model before any client can attach).
type engineBirth struct {
	Cols            int
	Rows            int
	ScrollbackLines int
	Blob            []byte
	Notice          []byte
}

// spawnShell starts a live shell session. The host theme is seeded at birth so the model
// answers OSC 10/11 truthfully before the process can ask. The caller admits the birth
// first, then registers the session, calls startReaper and settles the birth.
func (e *terminalEngine) spawnShell(ctx context.Context, id, shell, cwd, profileID string, b engineBirth) (*session.Session, error) {
	bg, fg := e.hostTheme()
	var (
		s   *session.Session
		err error
	)
	if b.Blob != nil {
		s, err = session.NewRestored(ctx, id, shell, cwd, profileID, ptyEnv(), b.Blob, session.WithTheme(bg, fg))
	} else {
		s, err = session.New(ctx, id, shell, cwd, profileID, ptyEnv(), b.Cols, b.Rows, b.ScrollbackLines,
			session.WithTheme(bg, fg))
	}
	if err != nil {
		return nil, err
	}
	if len(b.Notice) > 0 {
		s.InjectLocal(b.Notice)
	}
	return s, nil
}

// admit opens a birth BEFORE any child is started, refusing once Shutdown has begun
// draining: a session born after the kill loop has walked the registry would never be
// reaped. Every successful admit is paired with exactly one e.reaps.settle.
func (e *terminalEngine) admit() error {
	if e.reaps.admit() {
		return nil
	}
	return ErrShuttingDown
}

// startReaper watches s for its own death. The reaper runs under context.WithoutCancel of
// the birth context: a PTY is reaped when its process exits, long after the request that
// created it returned, so the request's cancellation must not reach it — but its values
// (trace/log scope) still describe the session's origin. It is joined by e.reaps, which
// Shutdown drains, not by the caller's ctx.
func (e *terminalEngine) startReaper(ctx context.Context, ent *sessionEntry, s *session.Session) {
	go e.reap(context.WithoutCancel(ctx), ent, s)
}

// reap waits for s to die and, if s is still the entry's live session, moves the entry to
// Exited. If a Kill or suspend already moved the entry on, it has nothing left to do.
func (e *terminalEngine) reap(ctx context.Context, ent *sessionEntry, s *session.Session) {
	// Registered FIRST so it runs LAST: even a panic recovered below must retire this
	// reap, or Shutdown's drain would wait on a goroutine that is already gone.
	defer e.reaps.done()
	defer safego.Recover("terminal.reap")
	<-s.Done()
	ent.mu.Lock()
	var fx effects
	if ent.state == stateLive && ent.sess.Load() == s {
		fx = e.endLocked(ctx, ent, stateExited, s.ExitCode())
	}
	ent.mu.Unlock()
	fx.run()
}

func (e *terminalEngine) Create(
	ctx context.Context,
	chatID string,
	workspaceDir string,
	prof *domain.TerminalProfile,
) (string, error) {
	resolved := profile.Resolve(prof, workspaceDir)
	id := uuid.NewString()

	if err := e.admit(); err != nil {
		return "", fmt.Errorf("terminal: create: %w", err)
	}
	// Create births at the historical 80×24 default; a fresh attach's first resize
	// reshapes both PTY and model.
	s, err := e.spawnShell(ctx, id, resolved.Shell, resolved.CWD, "", engineBirth{})
	if err != nil {
		e.reaps.settle(false)
		return "", fmt.Errorf("terminal: create: %w", err)
	}
	ent := &sessionEntry{
		id:     id,
		chatID: chatID,
		state:  stateLive,
		shell:  resolved.Shell,
		cwd:    resolved.CWD,
	}
	ent.sess.Store(s)
	e.register(ent)
	e.startReaper(ctx, ent, s)
	e.reaps.settle(true)

	for _, cmd := range resolved.Startup {
		if err := s.Write([]byte(cmd + "\n")); err != nil {
			// The PTY is alive; only the profile's startup command failed to reach it.
			slog.Warn("terminal: create: startup command not delivered", "session", id, "err", err)
			break
		}
	}
	return id, nil
}

// CreateCommand spawns an explicit argv+env as a registered session. onExit, if non-nil,
// is invoked exactly once when the session reaches Exited — see the Engine interface.
func (e *terminalEngine) CreateCommand(
	ctx context.Context,
	chatID string,
	cwd string,
	argv []string,
	env []string,
	onExit func(),
) (string, error) {
	// A vendor CLI whose reaper never runs is strictly worse than one never spawned: its
	// onExit is the ONLY thing that records the runner's death.
	if err := e.admit(); err != nil {
		return "", err
	}
	id := uuid.NewString()
	// CreateCommand takes the caller's env verbatim, so under launchd TERM and the locale
	// are absent; backfill the terminal defaults for any keys not already set.
	env = withTerminalDefaults(env)
	bg, fg := e.hostTheme()
	s, err := session.NewCommand(ctx, id, argv, cwd, env, 80, 24, 0, session.WithTheme(bg, fg))
	if err != nil {
		e.reaps.settle(false)
		// exec.ErrNotFound means argv[0] is not installed / not executable — a fact about
		// the USER'S MACHINE, not a server fault.
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("%w: %s", ErrCommandNotFound, argv[0])
		}
		return "", fmt.Errorf("terminal: create command: %w", err)
	}
	ent := &sessionEntry{
		id:      id,
		chatID:  chatID,
		command: true,
		state:   stateLive,
		shell:   s.Shell(),
		cwd:     cwd,
		onExit:  onExit,
	}
	ent.sess.Store(s)
	e.register(ent)
	e.startReaper(ctx, ent, s)
	e.reaps.settle(true)
	return id, nil
}

// LoadPlaceholder registers a persisted session in the Suspended state. Idempotent.
func (e *terminalEngine) LoadPlaceholder(_ context.Context, m SessionMeta, scrollback []byte) error {
	e.regMu.Lock()
	defer e.regMu.Unlock()
	if _, ok := e.entries[m.SessionID]; ok {
		return nil
	}
	ent := &sessionEntry{
		id:         m.SessionID,
		chatID:     m.ChatID,
		state:      stateSuspended,
		shell:      m.Shell,
		cwd:        m.CWD,
		profileID:  m.ProfileID,
		blob:       scrollback,
		lastActive: m.LastActiveAt,
	}
	e.entries[ent.id] = ent
	ids := e.byChat[ent.chatID]
	if ids == nil {
		ids = make(map[string]*sessionEntry)
		e.byChat[ent.chatID] = ids
	}
	ids[ent.id] = ent
	return nil
}

// ── transitions ──────────────────────────────────────────────────────────────

// endLocked moves ent to a terminal state (Exited after a process death, Removed for a
// suspended session dropped without one) and returns the callbacks that end owes. It is
// reached at most once per entry: every caller checks the current state under ent.mu, and
// this leaves the entry in a state none of them accepts. Caller holds ent.mu.
func (e *terminalEngine) endLocked(ctx context.Context, ent *sessionEntry, to sessionState, exitCode int) effects {
	e.unregister(ent)
	e.discardPersisted(ctx, ent)
	ent.state = to
	ent.sess.Store(nil)
	ent.blob = nil
	var fx effects
	if onExit := ent.onExit; onExit != nil {
		ent.onExit = nil
		fx = append(fx, onExit)
	}
	return append(fx, func() { e.fireEnded(ctx, ent.chatID, ent.id, exitCode) })
}

// restoreLocked moves a Suspended entry to Live by spawning a fresh shell from its saved
// screen. A saved cwd that no longer exists falls back to the home directory with an
// on-screen notice. A session that cannot be re-spawned even then is Removed (and reported
// ended) rather than left to fail every future Attach; one refused because the engine is
// shutting down stays Suspended, so the next daemon start restores it. Caller holds ent.mu.
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
	return effects{func() { e.fireState(ctx, ent.chatID, ent.id, "detached") }}, nil
}

// suspendLocked moves a Live, eligible entry to Suspended: it serializes the screen,
// persists it (.buf, then a "suspended" meta row), and only then kills the process. If the
// screen cannot be persisted the session stays Live — a suspended session with no .buf
// would restore blank. Caller holds ent.mu.
func (e *terminalEngine) suspendLocked(ctx context.Context, ent *sessionEntry, force bool) effects {
	if ent.state != stateLive || ent.command {
		return nil
	}
	s := ent.sess.Load()
	if !s.SuspendEligible(force) {
		return nil
	}
	var blob []byte
	if force && !s.IsIdle() {
		// Force-suspend of a LIVE app: teardown + notice + serialize in one session-lock
		// hold so no live chunk can re-alt the model between them (§11.2).
		blob = s.ForceSuspendSnapshot(
			[]byte("\r\n[crowbar] session suspended to free resources; re-open to restore\r\n"),
		)
	} else {
		blob, _ = s.Snapshot()
	}
	if err := e.writeBuf(ctx, ent, blob); err != nil {
		slog.Warn("terminal: suspend: keeping session live, screen not persisted", "session", ent.id, "err", err)
		return nil
	}
	e.parkLocked(ctx, ent, s, blob)
	return effects{func() { e.fireState(ctx, ent.chatID, ent.id, "suspended") }}
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

// TerminateGraceful sends the live process group a clean-exit SIGTERM (a PID-level action,
// never a PTY write) and waits up to the grace window for it to exit on its own before
// falling back to a hard kill. The wait happens OUTSIDE the lifecycle lock; whatever the
// session has become by the end of it (the reaper may already have recorded the exit) is
// then ended exactly like Kill.
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
	fx := e.suspendLocked(ctx, ent, force)
	ent.mu.Unlock()
	fx.run()
}

// Shutdown ends every session and deregisters it, then BLOCKS until every reaper has
// returned — i.e. until every onExit callback registered via CreateCommand has RUN.
//
// A live shell is persisted first (screen .buf + a "suspended" meta row) and left in the
// Suspended state, so the next daemon start restores it; its clients are NOT sent an exit
// frame, because it did not exit. A command session (an agentic vendor CLI) is not
// restorable: it is killed and Exited, so its onExit runs before Shutdown returns. Once
// Shutdown begins, the engine births no more sessions. Idempotent.
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

// resolveRestoreCWD returns a working directory guaranteed to exist for a restore spawn,
// plus an optional on-screen notice. If the saved cwd still resolves to a directory it is
// returned with no notice; otherwise it falls back to the user's home directory (or "" so
// the shell defaults) and returns a one-line notice injected into the model (never the
// persisted blob) so the user sees why their previous directory was not used.
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

// reapTracker counts the live reap goroutines and lets Shutdown wait them out. A reap is
// a WRITER — its onExit is how a dying vendor CLI's death is recorded — to databases the
// shutdown sequence is about to close, so the reap must be a step OF the shutdown.
//
// It is a counter + a mutex rather than a sync.WaitGroup because a WaitGroup PANICS on Add
// concurrent with Wait, and a session can be born while Shutdown waits. Making "admit a new
// reap" and "start draining" one critical section removes the race: once draining, admit()
// refuses, so the outstanding set only shrinks and the drain converges.
//
// births is held (read side) from admit to settle, i.e. across spawn and registration, so
// drain cannot close the door while a child is started but not yet visible to Shutdown's
// registry walk.
type reapTracker struct {
	births   sync.RWMutex
	mu       sync.Mutex
	n        int
	draining bool
	idle     chan struct{} // non-nil only while a drain is waiting on n > 0
}

// admit opens a birth and claims its reap slot, reporting false once a drain has begun.
// A true result must be followed by exactly one settle.
func (t *reapTracker) admit() bool {
	t.births.RLock()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.draining {
		t.births.RUnlock()
		return false
	}
	t.n++
	return true
}

// settle closes a birth opened by admit. A birth whose child never started (spawned ==
// false) returns its reap slot, since no reaper will ever retire it.
func (t *reapTracker) settle(spawned bool) {
	if !spawned {
		t.done()
	}
	t.births.RUnlock()
}

// done retires a reap goroutine, releasing a waiting drain once the last one is home.
func (t *reapTracker) done() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.n--
	if t.n == 0 && t.idle != nil {
		close(t.idle)
		t.idle = nil
	}
}

// drain closes the door on new reaps and returns a channel closed once every outstanding
// one has returned. Idempotent.
func (t *reapTracker) drain() <-chan struct{} {
	t.births.Lock()
	defer t.births.Unlock()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.draining = true
	if t.n == 0 {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	if t.idle == nil {
		t.idle = make(chan struct{})
	}
	return t.idle
}
