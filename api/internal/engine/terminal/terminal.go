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
	// session so the lifecycle topic can emit an "ended" frame.
	OnSessionEnded(
		fn func(ctx context.Context, workspaceID string, sessionID string),
	)

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

	// Stats returns a snapshot of session counts and total estimated ring-buffer
	// memory for observability (health endpoint, settings panel, etc.).
	Stats() (active, detached, suspended int, ringBytes int64)

	// Shutdown terminates all active sessions and removes them from the registry.
	Shutdown()
}

type terminalEngine struct {
	reg *registry.Registry

	mu         sync.RWMutex
	onEnded    func(ctx context.Context, workspaceID string, sessionID string)
	endedOnce  map[string]struct{}
	metaStore  SessionMetaStore
	lastActive map[string]time.Time // guarded by mu; set on last-client detach

	// restoring prevents double-spawn when concurrent Attach calls hit a
	// suspended placeholder. LoadOrStore ensures only the first caller spawns.
	restoring sync.Map // map[string]struct{}

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
	<-s.Done()

	// Suspend path: Suspend already replaced the registry entry with a placeholder
	// and persisted scrollback. We must NOT remove that placeholder or fire ended.
	if s.Suspending() {
		return
	}

	// Real exit (Kill, Shutdown, or PTY self-exit): full cleanup.
	e.reg.Remove(id)

	ctx := context.Background()
	dir, _ := e.storageDir(ctx, workspaceID)
	if dir != "" {
		if delErr := persistence.DeleteBuf(dir, id); delErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "terminal: reap: delete buf %s: %v\n", id, delErr)
		}
	}
	e.deleteMeta(ctx, id)
	e.fireEnded(ctx, workspaceID, id)
}

// fireEnded invokes the registered OnSessionEnded callback exactly once per
// session id, guarding against duplicate notifications.
func (e *terminalEngine) fireEnded(
	ctx context.Context,
	workspaceID string,
	sessionID string,
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

	fn(ctx, workspaceID, sessionID)
}

// OnSessionEnded registers the termination callback. The most recent
// registration wins, mirroring the LSP engine's OnDiagnostics hook.
func (e *terminalEngine) OnSessionEnded(
	fn func(ctx context.Context, workspaceID string, sessionID string),
) {
	e.mu.Lock()
	e.onEnded = fn
	e.mu.Unlock()
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
// It is idempotent: if the session is already live (or another goroutine is
// restoring it concurrently) it returns nil. The restoring sync.Map acts as a
// per-session spinlock that prevents double-spawn on concurrent first-attaches.
func (e *terminalEngine) restore(ctx context.Context, sid string) error {
	// Prevent double-spawn: only the first goroutine that wins LoadOrStore proceeds.
	if _, loaded := e.restoring.LoadOrStore(sid, struct{}{}); loaded {
		return nil
	}
	defer e.restoring.Delete(sid)

	s, ok := e.reg.Get(sid)
	if !ok || s.IsLive() {
		return nil // already gone or already restored by another path
	}

	ws, wsOK := e.reg.WorkspaceID(sid)
	if !wsOK {
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

	if _, err := e.spawn(sid, ws, shell, cwd, profileID, scrollback); err != nil {
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

	return nil
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
	scrollback := s.Snapshot()

	// For force-suspend of a running (non-idle) session, append a user-visible notice.
	if force && wasRunning {
		scrollback = append(scrollback,
			[]byte("\r\n[crowbar] session suspended to free resources; re-open to restore\r\n")...)
	}

	dir, _ := e.storageDir(ctx, ws)
	if dir != "" {
		if err := persistence.WriteBuf(dir, sid, scrollback); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "terminal: suspend: persist buf %s: %v\n", sid, err)
		}
	}
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
		now := time.Now()
		ws, wsOK := e.reg.WorkspaceID(sessionID)

		e.mu.Lock()
		e.lastActive[sessionID] = now
		e.mu.Unlock()

		if wsOK {
			dir, _ := e.storageDir(ctx, ws)
			if dir != "" {
				if writeErr := persistence.WriteBuf(dir, sessionID, s.Snapshot()); writeErr != nil {
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
		}
	}

	return nil
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
	_ context.Context,
	sessionID string,
) error {
	s, ok := e.reg.Get(sessionID)
	if !ok {
		return fmt.Errorf("terminal: kill: %w: %s", registry.ErrSessionNotFound, sessionID)
	}
	// Remove from registry eagerly so callers see it gone immediately after Kill returns.
	// reapOnDone will also call reg.Remove (idempotent no-op) and then handle the
	// remaining cleanup: deleteMeta, DeleteBuf, fireEnded.
	e.reg.Remove(sessionID)
	s.Kill()
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
		switch s.State() {
		case "active":
			active++
			ringBytes += int64(s.RingCap())
		case "detached":
			detached++
			ringBytes += int64(s.RingCap())
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
	// Phase 1: cadence flush
	// -------------------------------------------------------------------------
	for _, id := range e.reg.List() {
		s, ok := e.reg.Get(id)
		if !ok || !s.IsLive() {
			continue
		}
		if !s.TakeDirty() {
			continue
		}
		ws, wsOK := e.reg.WorkspaceID(id)
		if !wsOK {
			continue
		}
		dir, err := e.storageDir(ctx, ws)
		if err != nil || dir == "" {
			continue
		}
		snap := s.Snapshot()
		if writeErr := persistence.WriteBuf(dir, id, snap); writeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "terminal: maintenance: flush buf %s: %v\n", id, writeErr)
			continue
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

	underCeiling := func() bool {
		var lc int
		for _, id := range e.reg.List() {
			s, ok := e.reg.Get(id)
			if ok && s.IsLive() {
				lc++
			}
		}
		return lc <= maxTotalSessions && int64(lc)*int64(session.DefaultRingSize) <= maxTotalRingBytes
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

	// Phase 3b: last resort — force-suspend oldest detached running sessions.
	sort.Slice(runningDetached, func(i, j int) bool {
		return runningDetached[i].lastActive.Before(runningDetached[j].lastActive)
	})
	for _, c := range runningDetached {
		if underCeiling() {
			return
		}
		_ = e.suspend(ctx, c.id, true)
	}
}

// Shutdown terminates all active sessions and removes them from the registry.
func (e *terminalEngine) Shutdown() {
	e.stopOnce.Do(func() { close(e.stop) })
	for _, id := range e.reg.List() {
		s, ok := e.reg.Get(id)
		if !ok {
			continue
		}
		e.reg.Remove(id)
		s.Kill()
	}
}
