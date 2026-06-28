// Package terminal is the PTY session engine. It manages session lifecycle,
// ring-buffer replay, and multi-client WebSocket fan-out.
package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/persistence"
	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/profile"
	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/session"
)

// ptyEnv returns the process environment augmented with the terminal capability
// vars that a GUI-launched daemon won't inherit from any shell session. Without
// TERM, readline-based programs (bash, zsh, Claude Code, etc.) fall back to
// dumb-terminal mode and disable history navigation and line editing.
func ptyEnv() []string {
	base := os.Environ()
	overrides := map[string]string{
		"TERM":      "xterm-256color",
		"COLORTERM": "truecolor",
	}

	// Replace any existing TERM/COLORTERM entries, then append the rest.
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		keep := true
		for key := range overrides {
			if len(entry) > len(key) && entry[:len(key)+1] == key+"=" {
				keep = false
				break
			}
		}
		if keep {
			result = append(result, entry)
		}
	}
	for k, v := range overrides {
		result = append(result, k+"="+v)
	}
	return result
}

// WSConn is the WebSocket abstraction implemented by gorilla/websocket connections.
type WSConn interface {
	WriteMessage(
		messageType int,
		data []byte,
	) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

// ErrSessionNotFound is returned when an operation targets a session ID that
// does not exist in the registry.
var ErrSessionNotFound = registry.ErrSessionNotFound

// Engine is the full PTY session operation surface.
type Engine interface {
	// Create spawns a new PTY session in the given workspace directory.
	Create(
		ctx context.Context,
		workspaceID string,
		workspaceDir string,
		prof *domain.TerminalProfile,
	) (sessionID string, err error)

	// Attach connects a WebSocket connection to an existing session,
	// replaying the ring buffer and streaming live output. If the session is
	// suspended (placeholder), it is transparently restored before attaching.
	// The conn is closed by the engine when the session ends or the client is dropped.
	Attach(
		ctx context.Context,
		sessionID string,
		conn WSConn,
	) error

	// Write sends input bytes to the session's PTY stdin.
	Write(
		ctx context.Context,
		sessionID string,
		data []byte,
	) error

	// Resize sends SIGWINCH with the new terminal dimensions.
	Resize(
		ctx context.Context,
		sessionID string,
		cols uint16,
		rows uint16,
	) error

	// Kill terminates the session and deregisters it.
	Kill(
		ctx context.Context,
		sessionID string,
	) error

	// Suspend intentionally tears down an idle, unattached session's PTY,
	// persists its scrollback to disk, and replaces the registry entry with a
	// placeholder. A subsequent Attach transparently restores it. Returns nil
	// when the session is not eligible (has clients, not idle, or not live).
	Suspend(
		ctx context.Context,
		sessionID string,
	) error

	// ListSessions returns all active session IDs.
	ListSessions() []string

	// ListSessionsForWorkspace returns the active session IDs owned by the given
	// workspace.
	ListSessionsForWorkspace(
		workspaceID string,
	) []string

	// OnSessionEnded registers the callback invoked when a session terminates
	// (a Kill, a Shutdown, or a PTY self-exit). It fires exactly once per
	// session so the lifecycle topic can emit an "ended" frame. exitCode is
	// the process exit code captured by shutdown(); -1 if unknown (killed by
	// signal, placeholder, or daemon restart).
	OnSessionEnded(
		fn func(ctx context.Context, workspaceID string, sessionID string, exitCode int),
	)

	// OnSessionState registers the callback invoked when a session transitions
	// to "detached" (last client disconnects or post-restore) or "suspended"
	// (placeholder swap complete). The most recent registration wins.
	OnSessionState(
		fn func(ctx context.Context, workspaceID string, sessionID string, state string),
	)

	// StateOf returns the current state string ("active", "detached", or
	// "suspended") for the given session ID and true. Returns ("", false) when
	// the session does not exist in the registry.
	StateOf(sessionID string) (string, bool)

	// SessionExists reports whether a session with the given ID is currently active.
	// Returns true for both live sessions and suspended placeholders.
	SessionExists(
		ctx context.Context,
		sessionID string,
	) bool

	// SetMetaStore injects the durable session metadata store. It must be called
	// after both the engine and the terminal usecase are constructed to avoid an
	// import cycle (engine → usecase). A nil store is a valid no-op sentinel; all
	// internal meta helpers short-circuit when metaStore is nil.
	SetMetaStore(
		s SessionMetaStore,
	)

	// LoadPlaceholder loads a durable session from disk as a PTY-less placeholder
	// so a subsequent Attach can transparently restore it. It is idempotent: if the
	// session is already present in the registry (live or suspended), the call is a
	// no-op. No pump goroutine or reapOnDone is started — a later Attach→restore
	// handles that. Called by RestorePersistedSessions at daemon startup.
	LoadPlaceholder(
		ctx context.Context,
		m SessionMeta,
		scrollback []byte,
	) error

	// Stats returns a snapshot of session counts and total estimated ring-buffer
	// memory for observability (health endpoint, settings panel, etc.).
	Stats() (active, detached, suspended int, ringBytes int64)

	// Shutdown terminates all active sessions and removes them from the registry.
	Shutdown()
}

type terminalEngine struct {
	reg *registry.Registry

	mu         sync.RWMutex
	onEnded    func(ctx context.Context, workspaceID string, sessionID string, exitCode int)
	onState    func(ctx context.Context, workspaceID string, sessionID string, state string)
	endedOnce  map[string]struct{}
	metaStore  SessionMetaStore
	lastActive map[string]time.Time // guarded by mu; set on last-client detach

	// sessionMu serialises per-session lifecycle operations (restore, suspend,
	// Kill) so concurrent Attach calls on a placeholder never spawn two shells
	// or leave clients attached to a dead placeholder. Lock order:
	// sessionMu (outer) → s.mu (inner) → ring.mu. Never reverse.
	sessionMu sync.Map // map[string]*sync.Mutex

	stop     chan struct{}
	stopOnce sync.Once
}

var _ Engine = (*terminalEngine)(nil)

// Session limits — package-level so tests can override them.
var (
	softLimitPerWorkspace = 10               // max simultaneous detached sessions per workspace
	maxTotalSessions      = 100              // global hard ceiling (count)
	maxTotalRingBytes     = int64(256) << 20 // global hard ceiling (~ring bytes)
)

// MaxTotalSessions returns the global hard ceiling on the number of concurrent
// sessions (live plus suspended placeholders). RestorePersistedSessions uses it
// at daemon startup to cap how many persisted placeholders it reloads so a large
// on-disk backlog cannot blow the in-memory ceiling on restart.
func MaxTotalSessions() int {
	return maxTotalSessions
}

// New returns a new Engine.
func New() Engine {
	e := &terminalEngine{
		reg:        registry.New(),
		endedOnce:  make(map[string]struct{}),
		lastActive: make(map[string]time.Time),
		stop:       make(chan struct{}),
	}
	go e.maintenanceLoop()
	return e
}

// lockSession acquires the per-session lifecycle mutex for id and returns its
// unlock function. The caller must call the returned function (typically via
// defer) to release the lock. Lock order: sessionMu (outer) → s.mu (inner).
func (e *terminalEngine) lockSession(id string) func() {
	m, _ := e.sessionMu.LoadOrStore(id, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// outputMsg is the server→client wire frame.
type outputMsg struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
	IsInput   bool   `json:"isInput"`
}

// inputMsg is the client→server wire frame for PTY input.
type inputMsg struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// spawn creates a live session (or restores one from scrollback), registers it
// in the registry under id/workspaceID, and starts reapOnDone. It is the single
// point of session birth — both Create and restore delegate here.
func (e *terminalEngine) spawn(
	id string,
	workspaceID string,
	shell string,
	cwd string,
	profileID string,
	scrollback []byte,
) (*session.Session, error) {
	var (
		s   *session.Session
		err error
	)
	if len(scrollback) > 0 {
		s, err = session.NewRestored(id, shell, cwd, profileID, ptyEnv(), scrollback)
	} else {
		s, err = session.New(id, shell, cwd, profileID, ptyEnv())
	}
	if err != nil {
		return nil, err
	}
	e.reg.Add(id, workspaceID, s)
	go e.reapOnDone(id, workspaceID, s)
	return s, nil
}

func (e *terminalEngine) Create(
	_ context.Context,
	workspaceID string,
	workspaceDir string,
	prof *domain.TerminalProfile,
) (string, error) {
	resolved := profile.Resolve(prof, workspaceDir)
	id := uuid.NewString()

	s, err := e.spawn(id, workspaceID, resolved.Shell, resolved.CWD, "", nil)
	if err != nil {
		return "", fmt.Errorf("terminal: create: %w", err)
	}

	for _, cmd := range resolved.Startup {
		if writeErr := s.Write([]byte(cmd + "\n")); writeErr != nil {
			// Non-fatal: PTY is alive but startup command failed.
			break
		}
	}

	return id, nil
}

// reapOnDone removes the session from the registry once it terminates and fires
// the OnSessionEnded callback so the lifecycle topic can emit an "ended" frame.
// It covers every real-termination path: an explicit Kill, a Shutdown, or a PTY
// self-exit. For the suspend path it returns early so it does NOT remove the
// placeholder or fire ended.
func (e *terminalEngine) reapOnDone(
	id string,
	workspaceID string,
	s *session.Session,
) {
	defer safego.Recover("terminal.reapOnDone")

	// Wait for termination BEFORE acquiring the per-session lock so we never
	// block other lifecycle ops (Kill/suspend/flush/detach) while parked here.
	<-s.Done()

	// FIX: serialise the real-exit cleanup with the cadence-flush and
	// detach-bookkeeping persist paths via the per-session lifecycle lock. Lock
	// order: sessionMu (outer) → s.mu → ring.mu — we hold only sessionMu here.
	unlock := e.lockSession(id)

	// Suspend path: Suspend (or Shutdown) already replaced the registry entry
	// with a placeholder and persisted scrollback, or set the suspending flag.
	// We must NOT remove that placeholder, delete the .buf/row, or fire ended.
	// reg.Get(id) != s detects the placeholder swap; Suspending() covers the
	// shutdown/force paths. A removed entry (!ok, e.g. eager Kill of a live
	// session) is NOT the suspend case — fall through to real-exit cleanup,
	// which Kill delegates to reapOnDone for live sessions.
	if cur, ok := e.reg.Get(id); (ok && cur != s) || s.Suspending() {
		unlock()
		return
	}

	// Real exit (Kill, Shutdown-of-non-suspended, or PTY self-exit): full cleanup
	// under the lock. A flush/detach that lost the lock race sees !s.IsLive()
	// (ptmx nilled in shutdown) and skips; one that won wrote first and we delete
	// it here, so the explicitly-closed terminal never resurrects.
	exitCode := s.ExitCode()
	e.reg.Remove(id)

	ctx := context.Background()
	dir, _ := e.storageDir(ctx, workspaceID)
	if dir != "" {
		if delErr := persistence.DeleteBuf(dir, id); delErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "terminal: reap: delete buf %s: %v\n", id, delErr)
		}
	}
	e.deleteMeta(ctx, id)
	e.fireEnded(ctx, workspaceID, id, exitCode)

	// Prune per-session maps so they don't grow without bound.
	e.mu.Lock()
	delete(e.lastActive, id)
	delete(e.endedOnce, id)
	e.mu.Unlock()

	// Release the lifecycle lock before deleting its sessionMu entry, mirroring
	// Kill's ordering so a late lockSession(id) caller can never alias a freed mutex.
	unlock()
	e.sessionMu.Delete(id)
}

// fireEnded invokes the registered OnSessionEnded callback exactly once per
// session id, guarding against duplicate notifications. exitCode is the process
// exit code; -1 when unknown (killed by signal, placeholder, or daemon restart).
func (e *terminalEngine) fireEnded(
	ctx context.Context,
	workspaceID string,
	sessionID string,
	exitCode int,
) {
	e.mu.Lock()
	fn := e.onEnded
	if fn == nil {
		e.mu.Unlock()
		return
	}
	if _, done := e.endedOnce[sessionID]; done {
		e.mu.Unlock()
		return
	}
	e.endedOnce[sessionID] = struct{}{}
	e.mu.Unlock()

	fn(ctx, workspaceID, sessionID, exitCode)
}

// fireState invokes the registered OnSessionState callback if one is set.
// It is nil-safe: if no callback has been registered the call is a no-op.
func (e *terminalEngine) fireState(
	ctx context.Context,
	workspaceID string,
	sessionID string,
	state string,
) {
	e.mu.RLock()
	fn := e.onState
	e.mu.RUnlock()
	if fn == nil {
		return
	}
	fn(ctx, workspaceID, sessionID, state)
}

// OnSessionEnded registers the termination callback. The most recent
// registration wins, mirroring the LSP engine's OnDiagnostics hook.
func (e *terminalEngine) OnSessionEnded(
	fn func(ctx context.Context, workspaceID string, sessionID string, exitCode int),
) {
	e.mu.Lock()
	e.onEnded = fn
	e.mu.Unlock()
}

// OnSessionState registers the state-transition callback. The most recent
// registration wins. Fires on "detached" (last-client detach or post-restore)
// and "suspended" (placeholder swap). Does not fire for "ended" — use
// OnSessionEnded for that.
func (e *terminalEngine) OnSessionState(
	fn func(ctx context.Context, workspaceID string, sessionID string, state string),
) {
	e.mu.Lock()
	e.onState = fn
	e.mu.Unlock()
}

// StateOf returns the current state string for a session and true, or
// ("", false) when the session does not exist in the registry. The state is
// derived from the session's Session.State() method: "active", "detached", or
// "suspended".
func (e *terminalEngine) StateOf(sessionID string) (string, bool) {
	s, ok := e.reg.Get(sessionID)
	if !ok {
		return "", false
	}
	return s.State(), true
}

// SetMetaStore injects the durable session-metadata store. The most recent
// call wins. A nil argument removes any previously injected store; all
// internal meta helpers no-op when metaStore is nil.
func (e *terminalEngine) SetMetaStore(
	s SessionMetaStore,
) {
	e.mu.Lock()
	e.metaStore = s
	e.mu.Unlock()
}

// LoadPlaceholder loads a durable session into the registry as a PTY-less
// placeholder. Idempotent: if the session already exists in the registry (live
// or suspended), the call is a no-op and returns nil. No pump goroutine or
// reapOnDone is started — a later Attach→restore handles that.
func (e *terminalEngine) LoadPlaceholder(
	_ context.Context,
	m SessionMeta,
	scrollback []byte,
) error {
	// Idempotent: already in registry (live or placeholder) — skip.
	if _, ok := e.reg.Get(m.SessionID); ok {
		return nil
	}
	ph := session.NewPlaceholder(m.SessionID, m.Shell, m.CWD, m.ProfileID, scrollback)
	e.reg.Add(m.SessionID, m.WorkspaceID, ph)
	return nil
}

// saveMeta persists session metadata if a store is wired. It is a no-op when
// metaStore is nil, so callers do not need nil-guards at every call site.
func (e *terminalEngine) saveMeta(
	ctx context.Context,
	meta SessionMeta,
) {
	e.mu.RLock()
	ms := e.metaStore
	e.mu.RUnlock()
	if ms == nil {
		return
	}
	_ = ms.Save(ctx, meta)
}

// deleteMeta removes the session metadata row if a store is wired. It is a
// no-op when metaStore is nil.
func (e *terminalEngine) deleteMeta(
	ctx context.Context,
	sessionID string,
) {
	e.mu.RLock()
	ms := e.metaStore
	e.mu.RUnlock()
	if ms == nil {
		return
	}
	_ = ms.Delete(ctx, sessionID)
}

// storageDir resolves the per-workspace storage directory via the injected
// store. It returns ("", nil) when metaStore is nil.
func (e *terminalEngine) storageDir(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	e.mu.RLock()
	ms := e.metaStore
	e.mu.RUnlock()
	if ms == nil {
		return "", nil
	}
	return ms.StorageDir(ctx, workspaceID)
}

// restore spawns a live session to replace the placeholder currently at sid.
// It is idempotent: if the session is already live it returns nil immediately.
// The per-session lifecycle mutex (sessionMu) ensures that concurrent Attach
// calls on the same placeholder serialise here — the first caller spawns, all
// subsequent callers see IsLive()==true and return nil without a second spawn.
func (e *terminalEngine) restore(ctx context.Context, sid string) error {
	// Do NOT defer unlock here — the error path must release the lock before
	// deleting the sessionMu entry, mirroring reapOnDone's unlock-before-delete
	// ordering so a concurrent lockSession(sid) caller cannot alias a freed mutex.
	unlock := e.lockSession(sid)

	s, ok := e.reg.Get(sid)
	if !ok {
		unlock()
		return nil // session gone (concurrently killed)
	}
	if s.IsLive() {
		unlock()
		return nil // already restored by the goroutine that held the lock before us
	}

	ws, wsOK := e.reg.WorkspaceID(sid)
	if !wsOK {
		unlock()
		return fmt.Errorf("terminal: restore: workspace not found for session %s", sid)
	}

	shell := s.Shell()
	cwd := s.CWD()
	profileID := s.ProfileID()

	var scrollback []byte
	dir, dirErr := e.storageDir(ctx, ws)
	if dirErr == nil && dir != "" {
		scrollback, _ = persistence.ReadBuf(dir, sid)
	}

	// CWD fallback: the saved working directory may have been deleted (worktree
	// removed, repo cleaned) since suspend. Spawning with a non-existent cmd.Dir
	// fails, which — left unhandled — would strand the placeholder and make every
	// future Attach retry the same doomed spawn forever. Stat first and fall back
	// to the user's home directory, noting the substitution in the scrollback.
	cwd, scrollback = restoreCWD(cwd, scrollback)

	if _, err := e.spawn(sid, ws, shell, cwd, profileID, scrollback); err != nil {
		// Un-restorable even after the CWD fallback (e.g. the shell binary itself
		// is gone). Never leave the placeholder in the registry: drop it, delete
		// its persisted state, and fire ended so the FE removes the dead tab
		// instead of looping on Attach.
		//
		// Release the lifecycle lock before dropUnrestorable and the subsequent
		// sessionMu.Delete — matching reapOnDone's and Kill's unlock-before-delete
		// ordering so the entry is pruned while no lock is held.
		unlock()
		e.dropUnrestorable(ctx, ws, sid, dir)
		e.sessionMu.Delete(sid)
		return fmt.Errorf("terminal: restore: spawn: %w", err)
	}

	// Record restored session as detached (live but no clients yet).
	e.saveMeta(ctx, SessionMeta{
		SessionID:   sid,
		WorkspaceID: ws,
		CWD:         cwd,
		Shell:       shell,
		ProfileID:   profileID,
		State:       "detached",
	})
	e.fireState(ctx, ws, sid, "detached")

	unlock()
	return nil
}

// restoreCWD returns a working directory that is guaranteed to exist for a
// restore spawn. If the saved cwd still resolves to a directory it is returned
// unchanged; otherwise it falls back to the user's home directory (or "" so the
// shell defaults) and appends a one-line notice to the restored scrollback so
// the user can see why their previous directory was not used.
func restoreCWD(
	cwd string,
	scrollback []byte,
) (string, []byte) {
	if dirExists(cwd) {
		return cwd, scrollback
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	notice := fmt.Sprintf(
		"\r\n[crowbar] previous directory unavailable; restored in %s\r\n", home)
	return home, append(scrollback, []byte(notice)...)
}

// dirExists reports whether path resolves to an existing directory.
func dirExists(
	path string,
) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// dropUnrestorable permanently removes a placeholder whose restore spawn failed,
// so a doomed session can never loop forever on Attach. It removes the registry
// entry, deletes the persisted .buf and meta row, fires the ended callback so
// the FE drops the tab, and prunes the per-session bookkeeping maps. The caller
// holds the per-session lifecycle lock (sessionMu) for sid.
func (e *terminalEngine) dropUnrestorable(
	ctx context.Context,
	ws string,
	sid string,
	dir string,
) {
	e.reg.Remove(sid)
	if dir != "" {
		if delErr := persistence.DeleteBuf(dir, sid); delErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "terminal: restore: delete buf %s: %v\n", sid, delErr)
		}
	}
	e.deleteMeta(ctx, sid)
	e.fireEnded(ctx, ws, sid, -1)

	e.mu.Lock()
	delete(e.lastActive, sid)
	delete(e.endedOnce, sid)
	e.mu.Unlock()
}

// Suspend intentionally tears down a session's PTY if it is idle and has no
// attached clients. Returns nil for non-eligible sessions (not live, has clients,
// not idle) — the caller cannot distinguish a no-op from a success; detection is
// via the meta store.
func (e *terminalEngine) Suspend(ctx context.Context, sid string) error {
	return e.suspend(ctx, sid, false)
}

// suspend is the unified suspend implementation. force=false uses
// BeginSuspendIfEligible (idle-gated); force=true uses BeginForceSuspend
// (skips idle check, for detached-running last-resort). The persist →
// placeholder-swap → kill sequence is identical for both paths.
func (e *terminalEngine) suspend(ctx context.Context, sid string, force bool) error {
	// FIX 1: serialise with concurrent Kill / restore / Attach-triggered-restore
	// so the placeholder swap and kill form an atomic lifecycle transition.
	unlock := e.lockSession(sid)
	defer unlock()

	s, ok := e.reg.Get(sid)
	if !ok || !s.IsLive() {
		return nil
	}

	var wasRunning bool
	var began bool
	if force {
		wasRunning = !s.IsIdle() // capture before BeginForceSuspend sets suspending
		began = s.BeginForceSuspend()
	} else {
		began = s.BeginSuspendIfEligible()
	}
	if !began {
		return nil
	}

	ws, wsOK := e.reg.WorkspaceID(sid)
	if !wsOK {
		return nil
	}

	now := time.Now()

	// Step a: capture state + persist BEFORE replacing registry or killing.
	// FIX 2: hold flushMu across snapshot+write so this path serialises with
	// the cadence-flush, detach-bookkeeping, and shutdown flush paths.
	dir, _ := e.storageDir(ctx, ws)
	fm := s.FlushMu()
	fm.Lock()
	scrollback := s.Snapshot()
	// For force-suspend of a running (non-idle) session, append a user-visible notice.
	if force && wasRunning {
		scrollback = append(scrollback,
			[]byte("\r\n[crowbar] session suspended to free resources; re-open to restore\r\n")...)
	}
	if dir != "" {
		if err := persistence.WriteBuf(dir, sid, scrollback); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "terminal: suspend: persist buf %s: %v\n", sid, err)
		}
	}
	fm.Unlock()
	e.saveMeta(ctx, SessionMeta{
		SessionID:    sid,
		WorkspaceID:  ws,
		CWD:          s.CWD(),
		Shell:        s.Shell(),
		ProfileID:    s.ProfileID(),
		State:        "suspended",
		LastActiveAt: now,
	})

	// Step b: replace registry entry with placeholder BEFORE killing.
	ph := session.NewPlaceholder(sid, s.Shell(), s.CWD(), s.ProfileID(), scrollback)
	e.reg.Add(sid, ws, ph)

	// Step c: kill the OLD live session.
	s.Kill()

	// Notify lifecycle subscribers that the session is now suspended.
	e.fireState(ctx, ws, sid, "suspended")

	return nil
}

func (e *terminalEngine) Attach(
	ctx context.Context,
	sessionID string,
	conn WSConn,
) error {
	s, ok := e.reg.Get(sessionID)
	if !ok {
		return fmt.Errorf("terminal: attach: %w: %s", registry.ErrSessionNotFound, sessionID)
	}

	// Restore-aware: if the registry holds a placeholder, restore it first.
	if !s.IsLive() {
		if err := e.restore(ctx, sessionID); err != nil {
			return fmt.Errorf("terminal: attach: restore: %w", err)
		}
		// Re-fetch: restore replaced the placeholder with a new live session.
		s, ok = e.reg.Get(sessionID)
		if !ok {
			return fmt.Errorf("terminal: attach: %w: %s (post-restore)", registry.ErrSessionNotFound, sessionID)
		}
		if !s.IsLive() {
			return fmt.Errorf("terminal: attach: restore failed for %s", sessionID)
		}
	}

	ch, err := s.Attach()
	if err != nil {
		return fmt.Errorf("terminal: attach: %w", err)
	}

	writeDone := make(chan struct{})
	go e.writePump(conn, sessionID, ch, writeDone)
	e.readPump(ctx, conn, s)

	s.Detach(ch)
	<-writeDone

	// Detach bookkeeping: when the last client leaves, persist scrollback and
	// record "detached" meta so a future restore knows the last-known state.
	// This runs after <-writeDone so the WS is already closed — never blocks detach.
	if s.AttachedCount() == 0 {
		e.persistOnDetach(ctx, sessionID, s)
	}

	return nil
}

// persistOnDetach writes the last-known scrollback + "detached" meta when the
// final client leaves. It takes the per-session lifecycle lock and checks
// s.IsLive() first: if the session died (self-exit/Kill) while we were
// detaching, reapOnDone owns cleanup and persisting here would resurrect an
// explicitly-closed terminal — so we skip. Lock order: sessionMu (outer) →
// s.mu/flushMu → ring.mu.
func (e *terminalEngine) persistOnDetach(ctx context.Context, sessionID string, s *session.Session) {
	unlock := e.lockSession(sessionID)
	defer unlock()

	// Lost the race to reap (or to a suspend that swapped in a placeholder):
	// the reap/suspend path owns the .buf/row — do not write.
	if !s.IsLive() {
		return
	}
	ws, wsOK := e.reg.WorkspaceID(sessionID)
	if !wsOK {
		return
	}

	now := time.Now()
	e.mu.Lock()
	e.lastActive[sessionID] = now
	e.mu.Unlock()

	dir, _ := e.storageDir(ctx, ws)
	if dir != "" {
		// Hold flushMu across snapshot+write so concurrent flush paths
		// (cadence, suspend, shutdown) don't interleave writes.
		fm := s.FlushMu()
		fm.Lock()
		snap := s.Snapshot()
		writeErr := persistence.WriteBuf(dir, sessionID, snap)
		fm.Unlock()
		if writeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "terminal: detach: persist buf %s: %v\n", sessionID, writeErr)
		}
	}
	e.saveMeta(ctx, SessionMeta{
		SessionID:    sessionID,
		WorkspaceID:  ws,
		CWD:          s.CWD(),
		Shell:        s.Shell(),
		ProfileID:    s.ProfileID(),
		State:        "detached",
		LastActiveAt: now,
	})
	e.fireState(ctx, ws, sessionID, "detached")
}

// writePump reads from ch and forwards output frames to the WebSocket.
// When the write fails or ch closes, it closes the connection so readPump unblocks.
func (e *terminalEngine) writePump(
	conn WSConn,
	sessionID string,
	ch <-chan session.OutputFrame,
	done chan<- struct{},
) {
	defer safego.Recover("terminal.writePump")
	defer func() {
		_ = conn.Close()
		close(done)
	}()
	for frame := range ch {
		msg := outputMsg{
			SessionID: sessionID,
			Data:      string(frame.Data),
		}
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		if writeErr := conn.WriteMessage(websocket.TextMessage, data); writeErr != nil {
			return
		}
	}
}

// readPump reads client messages from the WebSocket and dispatches them.
func (e *terminalEngine) readPump(
	_ context.Context,
	conn WSConn,
	s *session.Session,
) {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var msg inputMsg
		if jsonErr := json.Unmarshal(raw, &msg); jsonErr != nil {
			continue
		}

		if msg.Type == "resize" {
			_ = s.Resize(msg.Cols, msg.Rows)
			continue
		}

		_ = s.Write([]byte(msg.Data))
	}
}

func (e *terminalEngine) Write(
	_ context.Context,
	sessionID string,
	data []byte,
) error {
	s, ok := e.reg.Get(sessionID)
	if !ok {
		return fmt.Errorf("terminal: write: %w: %s", registry.ErrSessionNotFound, sessionID)
	}
	return s.Write(data)
}

func (e *terminalEngine) Resize(
	_ context.Context,
	sessionID string,
	cols uint16,
	rows uint16,
) error {
	s, ok := e.reg.Get(sessionID)
	if !ok {
		return fmt.Errorf("terminal: resize: %w: %s", registry.ErrSessionNotFound, sessionID)
	}
	return s.Resize(cols, rows)
}

func (e *terminalEngine) Kill(
	ctx context.Context,
	sessionID string,
) error {
	// FIX 1: serialise with concurrent suspend / restore so Kill never races a
	// suspend that could resurrect the session in the registry after removal.
	unlock := e.lockSession(sessionID)

	s, ok := e.reg.Get(sessionID)
	if !ok {
		unlock()
		return fmt.Errorf("terminal: kill: %w: %s", registry.ErrSessionNotFound, sessionID)
	}

	// Capture whether this is a placeholder BEFORE modifying state. A placeholder
	// session has no live PTY (IsLive() == false) so no reapOnDone goroutine is
	// running to fire ended/cleanup after Kill returns.
	isPlaceholder := !s.IsLive()
	ws, wsOK := e.reg.WorkspaceID(sessionID)

	// Remove from registry eagerly so callers see it gone immediately after Kill returns.
	// For live sessions, reapOnDone will also call reg.Remove (idempotent no-op) and then
	// handle the remaining cleanup: deleteMeta, DeleteBuf, fireEnded.
	e.reg.Remove(sessionID)
	s.Kill()

	// For placeholder sessions (suspended state), no reapOnDone goroutine is listening
	// on s.Done(). Perform the cleanup and fire the ended callback inline.
	if isPlaceholder && wsOK {
		exitCode := s.ExitCode() // -1 for placeholders (no process)
		dir, _ := e.storageDir(ctx, ws)
		if dir != "" {
			if delErr := persistence.DeleteBuf(dir, sessionID); delErr != nil {
				_, _ = fmt.Fprintf(os.Stderr, "terminal: kill: delete buf %s: %v\n", sessionID, delErr)
			}
		}
		e.deleteMeta(ctx, sessionID)
		e.fireEnded(ctx, ws, sessionID, exitCode)

		// FIX 4: prune per-session maps. For live sessions, reapOnDone handles
		// this after the PTY exits. For placeholders, Kill is the terminal path.
		e.mu.Lock()
		delete(e.lastActive, sessionID)
		delete(e.endedOnce, sessionID)
		e.mu.Unlock()
	}

	// Release the session lock, then remove the sessionMu entry for placeholders.
	// Live-session sessionMu cleanup is deferred to reapOnDone.
	unlock()
	if isPlaceholder {
		e.sessionMu.Delete(sessionID)
	}

	return nil
}

func (e *terminalEngine) ListSessions() []string {
	return e.reg.List()
}

// ListSessionsForWorkspace returns the active session IDs owned by workspaceID.
func (e *terminalEngine) ListSessionsForWorkspace(
	workspaceID string,
) []string {
	return e.reg.ListByWorkspace(workspaceID)
}

// SessionExists reports whether a session with the given ID is currently active.
// Returns true for both live sessions and suspended placeholders.
func (e *terminalEngine) SessionExists(
	_ context.Context,
	sessionID string,
) bool {
	_, ok := e.reg.Get(sessionID)
	return ok
}

// Stats returns a point-in-time snapshot of session counts and estimated
// ring-buffer memory across all registered sessions.
func (e *terminalEngine) Stats() (active, detached, suspended int, ringBytes int64) {
	for _, id := range e.reg.List() {
		s, ok := e.reg.Get(id)
		if !ok {
			continue
		}
		// Count every session's actual ring capacity — live sessions pin a full
		// 256 KB ring, suspended placeholders only their lazily-sized scrollback —
		// so the total reflects real resident memory across mixed session types.
		ringBytes += int64(s.RingCap())
		switch s.State() {
		case "active":
			active++
		case "detached":
			detached++
		case "suspended":
			suspended++
		}
	}
	return
}

// getLastActive returns the stored last-active time for a session, or the zero
// time if the session has never had a client detach recorded.
func (e *terminalEngine) getLastActive(id string) time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastActive[id]
}

// allWorkspaceIDs returns the distinct workspace IDs present in the registry.
func (e *terminalEngine) allWorkspaceIDs() []string {
	all := e.reg.List()
	seen := make(map[string]struct{}, len(all))
	var out []string
	for _, id := range all {
		ws, ok := e.reg.WorkspaceID(id)
		if !ok {
			continue
		}
		if _, dup := seen[ws]; !dup {
			seen[ws] = struct{}{}
			out = append(out, ws)
		}
	}
	return out
}

// maintenanceLoop runs runMaintenanceOnce on a 10-second ticker until the stop
// channel is closed by Shutdown.
func (e *terminalEngine) maintenanceLoop() {
	defer safego.Recover("terminal.maintenanceLoop")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-ticker.C:
			e.runMaintenanceOnce(context.Background())
		}
	}
}

// sessionCandidate pairs a session ID with its last-active time for sorting.
type sessionCandidate struct {
	id         string
	lastActive time.Time
}

// flushSessionOnce persists one session's dirty ring snapshot under the
// per-session lifecycle lock. Acquiring e.lockSession(id) serialises this write
// with reapOnDone (delete) and the detach-bookkeeping persist, and the
// !s.IsLive() guard makes a flush that loses the race to reap a no-op — so a
// cadence flush can never resurrect the .buf/row of an explicitly-closed
// terminal. Lock order: sessionMu (outer) → s.mu/flushMu → ring.mu.
func (e *terminalEngine) flushSessionOnce(ctx context.Context, id string) {
	unlock := e.lockSession(id)
	defer unlock()

	s, ok := e.reg.Get(id)
	if !ok || !s.IsLive() {
		// Dead or gone: reapOnDone owns cleanup; never re-write its deleted .buf.
		return
	}
	if !s.TakeDirty() {
		return
	}
	ws, wsOK := e.reg.WorkspaceID(id)
	if !wsOK {
		return
	}
	dir, err := e.storageDir(ctx, ws)
	if err != nil || dir == "" {
		return
	}
	// Hold flushMu across snapshot+write so concurrent flush paths
	// (detach-bookkeeping, suspend, shutdown) don't interleave.
	fm := s.FlushMu()
	fm.Lock()
	snap := s.Snapshot()
	writeErr := persistence.WriteBuf(dir, id, snap)
	fm.Unlock()
	if writeErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "terminal: maintenance: flush buf %s: %v\n", id, writeErr)
		return
	}
	e.saveMeta(ctx, SessionMeta{
		SessionID:    id,
		WorkspaceID:  ws,
		CWD:          s.CWD(),
		Shell:        s.Shell(),
		ProfileID:    s.ProfileID(),
		State:        s.State(),
		LastActiveAt: e.getLastActive(id),
	})
}

// runMaintenanceOnce performs the three-phase maintenance sweep in order:
//
//  1. Cadence flush — for each LIVE session with new output (dirty), persist
//     the ring snapshot and save updated meta.
//
//  2. Per-workspace soft limit — if a workspace has more than
//     softLimitPerWorkspace detached sessions, idle-gated-suspend the oldest
//     idle detached ones until back at/under the limit.
//
//  3. Global ceiling — if over maxTotalSessions or maxTotalRingBytes:
//     a. first idle-gated-suspend oldest idle detached sessions;
//     b. as a last resort, force-suspend oldest detached running sessions.
//
// This method is called on the ticker and exposed to tests via
// RunMaintenanceOnceForTest so tests can drive it directly without waiting 10 s.
func (e *terminalEngine) runMaintenanceOnce(ctx context.Context) {
	// -------------------------------------------------------------------------
	// Phase 1: cadence flush — acquire/release the per-session lock per id so
	// the sweep never holds it across the whole registry iteration.
	// -------------------------------------------------------------------------
	for _, id := range e.reg.List() {
		e.flushSessionOnce(ctx, id)
	}

	// -------------------------------------------------------------------------
	// Phase 2: per-workspace soft limit (idle-gated only)
	// -------------------------------------------------------------------------
	for _, ws := range e.allWorkspaceIDs() {
		wsIDs := e.reg.ListByWorkspace(ws)

		var totalDetached int
		var idleCandidates []sessionCandidate
		for _, id := range wsIDs {
			s, ok := e.reg.Get(id)
			if !ok || !s.IsLive() {
				continue
			}
			if s.AttachedCount() > 0 {
				continue // attached — never touch
			}
			totalDetached++
			if s.IsIdle() {
				idleCandidates = append(idleCandidates, sessionCandidate{
					id:         id,
					lastActive: e.getLastActive(id),
				})
			}
		}

		if totalDetached <= softLimitPerWorkspace {
			continue
		}

		// Sort oldest-first and suspend the excess.
		sort.Slice(idleCandidates, func(i, j int) bool {
			return idleCandidates[i].lastActive.Before(idleCandidates[j].lastActive)
		})
		excess := totalDetached - softLimitPerWorkspace
		for i := 0; i < excess && i < len(idleCandidates); i++ {
			_ = e.Suspend(ctx, idleCandidates[i].id)
		}
	}

	// -------------------------------------------------------------------------
	// Phase 3: global ceiling
	// -------------------------------------------------------------------------
	// Snapshot which sessions are live and classify detached ones.
	var idleGlobal []sessionCandidate
	var runningDetached []sessionCandidate

	for _, id := range e.reg.List() {
		s, ok := e.reg.Get(id)
		if !ok || !s.IsLive() {
			continue
		}
		if s.AttachedCount() > 0 {
			continue // attached — never touch
		}
		la := e.getLastActive(id)
		if s.IsIdle() {
			idleGlobal = append(idleGlobal, sessionCandidate{id, la})
		} else {
			runningDetached = append(runningDetached, sessionCandidate{id, la})
		}
	}

	// underCeiling counts EVERY registered session — live and suspended
	// placeholder alike — and sums each one's actual ring capacity, so an
	// unbounded pile of suspended placeholders can no longer slip past the
	// global memory/count ceiling uncounted.
	underCeiling := func() bool {
		var count int
		var bytes int64
		for _, id := range e.reg.List() {
			s, ok := e.reg.Get(id)
			if !ok {
				continue
			}
			count++
			bytes += int64(s.RingCap())
		}
		return count <= maxTotalSessions && bytes <= maxTotalRingBytes
	}

	if underCeiling() {
		return
	}

	// Phase 3a: idle-gated suspend.
	sort.Slice(idleGlobal, func(i, j int) bool {
		return idleGlobal[i].lastActive.Before(idleGlobal[j].lastActive)
	})
	for _, c := range idleGlobal {
		if underCeiling() {
			return
		}
		_ = e.Suspend(ctx, c.id)
	}

	// Phase 3b: force-suspend oldest detached running sessions.
	sort.Slice(runningDetached, func(i, j int) bool {
		return runningDetached[i].lastActive.Before(runningDetached[j].lastActive)
	})
	for _, c := range runningDetached {
		if underCeiling() {
			return
		}
		_ = e.suspend(ctx, c.id, true)
	}

	// Phase 3c: last resort — evict oldest suspended placeholders (LRU). Suspend
	// only converts a live ring to a placeholder ring; it never lowers the
	// session COUNT, so once placeholders themselves push us over the ceiling the
	// only way back under is to drop the least-recently-active ones entirely.
	if underCeiling() {
		return
	}
	placeholders := e.placeholderCandidates()
	sort.Slice(placeholders, func(i, j int) bool {
		return placeholders[i].lastActive.Before(placeholders[j].lastActive)
	})
	for _, c := range placeholders {
		if underCeiling() {
			return
		}
		e.evictPlaceholder(ctx, c.id)
	}
}

// placeholderCandidates returns the suspended (non-live) sessions paired with
// their last-active time, for LRU eviction ordering.
func (e *terminalEngine) placeholderCandidates() []sessionCandidate {
	var out []sessionCandidate
	for _, id := range e.reg.List() {
		s, ok := e.reg.Get(id)
		if !ok || s.IsLive() {
			continue
		}
		out = append(out, sessionCandidate{id, e.getLastActive(id)})
	}
	return out
}

// evictPlaceholder permanently drops a suspended placeholder to reclaim its
// registry slot, scrollback file, and meta row when the global ceiling is
// exceeded. It takes the per-session lifecycle lock and re-checks that the
// session is still a placeholder (a concurrent Attach may have restored it to
// live), then removes it from the registry, deletes its .buf and meta row, and
// prunes the per-session bookkeeping maps. Lock order: sessionMu (outer) →
// s.mu → ring.mu — we hold only sessionMu here.
func (e *terminalEngine) evictPlaceholder(
	ctx context.Context,
	id string,
) {
	unlock := e.lockSession(id)

	s, ok := e.reg.Get(id)
	if !ok || s.IsLive() {
		unlock()
		return
	}
	ws, _ := e.reg.WorkspaceID(id)
	e.reg.Remove(id)

	dir, _ := e.storageDir(ctx, ws)
	if dir != "" {
		if delErr := persistence.DeleteBuf(dir, id); delErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "terminal: evict: delete buf %s: %v\n", id, delErr)
		}
	}
	e.deleteMeta(ctx, id)

	e.mu.Lock()
	delete(e.lastActive, id)
	delete(e.endedOnce, id)
	e.mu.Unlock()

	unlock()
	e.sessionMu.Delete(id)
}

// Shutdown terminates all active sessions and removes them from the registry.
//
// Graceful-shutdown contract (Phase 3):
//   - For each LIVE session, before killing, Shutdown flushes the ring-buffer
//     snapshot to disk (persistence.WriteBuf) and saves meta with
//     state="suspended" so a subsequent daemon start can call
//     RestorePersistedSessions and hand the session back as a placeholder.
//   - s.MarkSuspendingForShutdown() is called before s.Kill() so that
//     reapOnDone takes its early-return path and does NOT delete the .buf file
//     or meta row — the delete-on-reap path is reserved for explicit Kill()
//     calls during normal operation, not daemon shutdown.
//   - Placeholder sessions (already suspended, no live PTY) are killed without
//     any additional flush because their scrollback is already on disk.
//   - All persist operations are best-effort: errors are logged but never cause
//     Shutdown to panic or hang.
func (e *terminalEngine) Shutdown() {
	e.stopOnce.Do(func() { close(e.stop) })
	ctx := context.Background()
	for _, id := range e.reg.List() {
		s, ok := e.reg.Get(id)
		if !ok {
			continue
		}
		if s.IsLive() {
			ws, wsOK := e.reg.WorkspaceID(id)
			if wsOK {
				// a) Flush scrollback to disk (best-effort; continue on error).
				// FIX 2: hold flushMu across snapshot+write to serialise with
				// any concurrent cadence-flush or detach-bookkeeping flush.
				dir, _ := e.storageDir(ctx, ws)
				if dir != "" {
					fm := s.FlushMu()
					fm.Lock()
					snap := s.Snapshot()
					shutErr := persistence.WriteBuf(dir, id, snap)
					fm.Unlock()
					if shutErr != nil {
						_, _ = fmt.Fprintf(os.Stderr, "terminal: shutdown: persist buf %s: %v\n", id, shutErr)
					}
				}
				// b) Persist meta with state="suspended" for restart restore.
				e.saveMeta(ctx, SessionMeta{
					SessionID:    id,
					WorkspaceID:  ws,
					CWD:          s.CWD(),
					Shell:        s.Shell(),
					ProfileID:    s.ProfileID(),
					State:        "suspended",
					LastActiveAt: time.Now(),
				})
			}
			// c) Mark suspending so reapOnDone skips the delete-on-reap path.
			s.MarkSuspendingForShutdown()
		}
		// d) Remove from registry and kill (process dies; scrollback survives).
		e.reg.Remove(id)
		s.Kill()
	}
}
