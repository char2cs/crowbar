// Package terminal is the PTY session engine. It owns every session's lifecycle as an
// explicit state machine (lifecycle.go), persists and bounds sessions (persist.go), and
// streams a session to its WebSocket clients (transport.go).
package terminal

import (
	"context"
	"errors"
	"image/color"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// WSConn is the WebSocket abstraction implemented by gorilla/websocket connections.
type WSConn interface {
	WriteMessage(
		messageType int,
		data []byte,
	) error
	ReadMessage() (messageType int, p []byte, err error)
	// SetWriteDeadline bounds every write, so a client that stops reading cannot
	// park the engine's writer — and with it Attach — forever.
	SetWriteDeadline(t time.Time) error
	Close() error
}

// ErrSessionNotFound is returned when an operation targets a session ID that is
// not registered (never existed, or has ended).
var ErrSessionNotFound = errors.New("terminal: session not found")

// ErrShuttingDown is returned by the session-birth paths (Create, CreateCommand,
// and the restore inside Attach) once Shutdown has begun draining. Birthing a
// session after that point would hand back a PTY nothing is left to reap: the
// kill loop has already walked the registry, so the process would outlive the
// daemon and its exit callback would fire into a torn-down app.
var ErrShuttingDown = errors.New("terminal: engine is shutting down")

// ErrCommandNotFound is returned by CreateCommand when argv[0] names a binary that
// does not exist or is not executable. It is a MISSING DEPENDENCY on the user's
// machine, not a server fault, and it is classified here — where the exec actually
// fails — so callers never have to string-match exec's error text to tell "this CLI
// is not installed" apart from a genuine spawn failure.
var ErrCommandNotFound = errors.New("terminal: command not found")

// Engine is the full PTY session operation surface.
type Engine interface {
	// Create spawns a new PTY session owned by chatID, running in
	// workspaceDir.
	//
	// The two arguments answer DIFFERENT questions and must not be conflated.
	// chatID is the session's OWNER: it is what the session is registered,
	// listed, and reaped under, and it is why a chat never sees a sibling's
	// shells. workspaceDir is merely where the shell starts — the resolved
	// worktree path, which sibling chats routinely SHARE. One worktree, many
	// owners.
	Create(
		ctx context.Context,
		chatID string,
		workspaceDir string,
		prof *domain.TerminalProfile,
	) (sessionID string, err error)

	// CreateCommand spawns an explicit argv+env as a registered session (streamable
	// over the terminal WS), skipping profile resolution. Used by the agentic
	// engine to launch vendor CLIs (claude/codex) with descriptor-built argv/env.
	// onExit, if non-nil, is invoked exactly once — after the session is fully
	// reaped (natural PTY exit or an explicit Kill) — so a caller can release
	// resources (e.g. the per-spawn hook-config tmp dir) that must stay alive for
	// the whole lifetime of the running CLI. It is never called for a session
	// that is merely suspended/detached.
	CreateCommand(
		ctx context.Context,
		chatID string,
		cwd string,
		argv []string,
		env []string,
		onExit func(),
	) (sessionID string, err error)

	// Attach connects a WebSocket connection to an existing session, sending the
	// serialized screen-model snapshot to redraw the client and then streaming
	// live output. A SUSPENDED session is transparently restored first; a session
	// whose process has exited never is (see sessionState). When the process
	// exits, the client is sent an exit frame before the conn is closed, so it can
	// tell "the shell ended" from "the transport dropped". Attach returns within a
	// bounded time once its client stalls (every write carries a deadline) or the
	// session dies.
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

	// Kill terminates the session's process group (or drops a suspended session)
	// and deregisters it. It returns after the process has been reaped and the
	// ended callback (and a CreateCommand onExit) has run.
	Kill(
		ctx context.Context,
		sessionID string,
	) error

	// TerminateGraceful gracefully quits a running vendor CLI (spec §8): it
	// sends a clean-exit SIGTERM — a PID-level action, never a PTY write —
	// and waits up to an internal grace window before falling back to a hard
	// Kill if the process hasn't exited on its own. Used by provider switches
	// so a well-behaved CLI (e.g. Claude Code) gets the chance to flush its
	// native transcript on a clean exit — a hard SIGKILL can lose the
	// outgoing CLI's last pre-switch turn.
	TerminateGraceful(
		ctx context.Context,
		sessionID string,
	) error

	// Suspend intentionally tears down an idle, unattached session's PTY and
	// persists its scrollback to disk, moving the session to the suspended state.
	// A subsequent Attach transparently restores it. Returns nil when the session
	// is not eligible (has clients, not idle, not live, or a command session).
	Suspend(
		ctx context.Context,
		sessionID string,
	) error

	// SetHostTheme records the host terminal's default background/foreground colours, which
	// every session born afterwards answers an OSC 10/11 query with from the moment it
	// exists. See the implementation for why this is separate from the per-session
	// Session.SetTheme push.
	SetHostTheme(
		bg color.Color,
		fg color.Color,
	)

	// ListSessions returns all active session IDs.
	ListSessions() []string

	// ListSessionsForChat returns the active session IDs owned by the given
	// chat — and ONLY that chat's. A sibling chat sharing the same worktree
	// gets its own, disjoint answer.
	ListSessionsForChat(
		chatID string,
	) []string

	// OnSessionEnded registers the callback invoked when a session ends for good
	// (a Kill, a PTY self-exit, an eviction, an unrestorable restore, or the
	// Shutdown of a command session). It fires exactly once per session, never
	// under an engine lock, so the lifecycle topic can emit an "ended" frame.
	// exitCode is the process exit code; -1 if unknown (killed by signal, or the
	// session was suspended with no process).
	OnSessionEnded(
		fn func(ctx context.Context, chatID, sessionID string, exitCode int),
	)

	// OnSessionState registers the callback invoked when a session transitions
	// to "detached" (last client disconnects or post-restore) or "suspended".
	// The most recent registration wins.
	OnSessionState(
		fn func(ctx context.Context, chatID, sessionID, state string),
	)

	// StateOf returns the current state string ("active", "detached", or
	// "suspended") for the given session ID and true. Returns ("", false) when
	// the session does not exist in the registry.
	StateOf(sessionID string) (string, bool)

	// SessionExists reports whether a session with the given ID is registered:
	// true for both live and suspended sessions.
	SessionExists(
		ctx context.Context,
		sessionID string,
	) bool

	// SessionLive reports whether a session id is backed by a LIVE PTY right now
	// — the honest process-liveness question, which SessionExists deliberately
	// does not answer (it is also true for a PTY-less suspended placeholder,
	// whose process is already dead and whose only remaining substance is
	// scrollback on disk). Callers that must know "is the process I spawned
	// still running" — the agent boot reconcile, which decides whether a chat's
	// vendor CLI survived a daemon restart — must use this, never SessionExists.
	SessionLive(
		ctx context.Context,
		sessionID string,
	) bool

	// Screen renders a session's VISIBLE screen as plain text, and reports whether
	// it has moved since generation `since` — pass 0 to always get the text.
	//
	// It is the daemon's own read of what a hosted program is showing, which is
	// what makes screen-derived facts server-side facts: an agent CLI blocked on a
	// modal is detected against this model, not scraped out of a browser that may
	// not even have the chat open.
	//
	// It is a PULL, and deliberately not an Attach. Attach is a subscription with
	// side effects — it restores a suspended placeholder, flushes pending emits and
	// re-primes the differ, and makes a detached session active for lifecycle
	// purposes — none of which an observer should cause merely by looking. This
	// takes s.mu, reads cells, and leaves the session exactly as it found it.
	//
	// A session that does not exist, is a suspended placeholder, or whose model
	// backend cannot render text reports ("", 0, false) — indistinguishable from
	// "nothing to see", which is what every caller does with it anyway.
	Screen(
		sessionID string,
		since uint64,
	) (text string, gen uint64, changed bool)

	// SetMetaStore injects the durable session metadata store. It must be called
	// after both the engine and the terminal usecase are constructed to avoid an
	// import cycle (engine → usecase). A nil store is a valid no-op sentinel; all
	// internal meta helpers short-circuit when metaStore is nil.
	SetMetaStore(
		s SessionMetaStore,
	)

	// LoadPlaceholder registers a durable session from disk in the suspended
	// state so a subsequent Attach can transparently restore it. It is idempotent:
	// if the session is already registered, the call is a no-op. Called by
	// RestorePersistedSessions at daemon startup.
	LoadPlaceholder(
		ctx context.Context,
		m SessionMeta,
		scrollback []byte,
	) error

	// Stats returns a snapshot of session counts, total estimated model memory, and
	// the parse-health surface (§9.4) for observability (health endpoint, settings
	// panel, etc.): how many live sessions are in the sticky degraded state and the
	// aggregate recovered-parse-panic count across all sessions.
	Stats() (active, detached, suspended int, modelBytes int64, degraded, parsePanics int)

	// Shutdown terminates all active sessions and removes them from the registry,
	// and BLOCKS until every session it killed has been fully reaped — i.e. until
	// every onExit callback registered via CreateCommand has RUN to completion.
	//
	// That join is a contract, not an implementation detail. onExit is where a dying
	// vendor CLI's death gets recorded (the agent usecase Exits its runner and closes
	// the turn the CLI abandoned), and it runs on the engine's own reap goroutine.
	// A caller that closed the databases without it would be closing them underneath
	// their own writers: the runner's Exit would commit while the turn's close would
	// not, leaving a chat spinning forever with no live runner left for the next
	// boot's reconcile to find. So the shutdown sequence must Shutdown the terminal
	// engine BEFORE it drains the aggregates and closes the DBs those callbacks write
	// to (engine.Container.QuiesceTerminal, called from app.Container.Shutdown).
	//
	// Once it begins, the engine births no more sessions: Create/CreateCommand (and
	// the restore inside Attach) return ErrShuttingDown. Idempotent.
	Shutdown()
}

// config is every tunable the engine reads. It is a value on the engine, not a set of
// package variables, so a test can shrink a limit or a clock for ONE engine without
// racing every other engine's background goroutines.
type config struct {
	softLimitPerChat   int           // max simultaneous detached sessions per chat
	maxTotalSessions   int           // global hard ceiling (count), live + suspended
	maxTotalModelBytes int64         // global hard ceiling (~serialized model bytes)
	maintenanceTick    time.Duration // cadence of the background maintenance sweep
	terminateGrace     time.Duration // TerminateGraceful's SIGTERM→SIGKILL window
	writeWait          time.Duration // deadline on every WebSocket write
}

const defaultMaxTotalSessions = 100

func defaultConfig() config {
	return config{
		softLimitPerChat:   10,
		maxTotalSessions:   defaultMaxTotalSessions,
		maxTotalModelBytes: int64(256) << 20,
		maintenanceTick:    10 * time.Second,
		// ~3s is enough for a well-behaved CLI to flush and exit without making a
		// provider switch feel stuck on a wedged/signal-ignoring process.
		terminateGrace: 3 * time.Second,
		writeWait:      10 * time.Second,
	}
}

// MaxTotalSessions returns the global hard ceiling on the number of concurrent
// sessions (live plus suspended). RestorePersistedSessions uses it at daemon
// startup to cap how many persisted sessions it reloads so a large on-disk
// backlog cannot blow the in-memory ceiling on restart.
func MaxTotalSessions() int {
	return defaultMaxTotalSessions
}

type terminalEngine struct {
	cfg config

	// regMu guards the registry maps. Lock order: sessionEntry.mu → regMu; a
	// registry reader never takes an entry lock while holding regMu.
	regMu   sync.RWMutex
	entries map[string]*sessionEntry
	byChat  map[string]map[string]*sessionEntry

	cbMu      sync.RWMutex
	onEnded   func(ctx context.Context, chatID, sessionID string, exitCode int)
	onState   func(ctx context.Context, chatID, sessionID, state string)
	metaStore SessionMetaStore

	// themeMu guards the host theme. themeBg/themeFg are the HOST TERMINAL's default
	// colours — one truth for every session in the window — held on the engine because
	// they must be known BEFORE a session exists: a vendor CLI that detects its theme by
	// querying the background does so within milliseconds of exec. Nil means unknown,
	// and unknown stays unknown (the emulator keeps its own default).
	themeMu          sync.RWMutex
	themeBg, themeFg color.Color

	// reaps tracks the reap goroutines so Shutdown can JOIN them: every exit
	// callback the engine still owes has RUN by the time Shutdown returns.
	reaps reapTracker

	stop     chan struct{}
	stopOnce sync.Once
	// maintDone is closed by maintenanceLoop when it returns, so Shutdown can JOIN it.
	maintDone chan struct{}
}

var _ Engine = (*terminalEngine)(nil)

// New returns a new Engine.
func New() Engine {
	return newEngine(defaultConfig())
}

func newEngine(cfg config) *terminalEngine {
	e := &terminalEngine{
		cfg:       cfg,
		entries:   make(map[string]*sessionEntry),
		byChat:    make(map[string]map[string]*sessionEntry),
		stop:      make(chan struct{}),
		maintDone: make(chan struct{}),
	}
	go e.maintenanceLoop()
	return e
}

// SetHostTheme records the host terminal's default background/foreground colours as the
// values every SUBSEQUENTLY BORN session answers an OSC 10/11 query with.
//
// It is the "future sessions" half of theme propagation; Session.SetTheme (driven by the
// per-session theme frame) is the "sessions that already exist" half. Without this, a
// session is born answering x/vt's hardcoded black, and an app that queries once at
// startup and never subscribes to DEC 2031 latches the wrong theme for its lifetime.
// A nil channel is a no-op for that channel, matching model.SetDefaultColors.
func (e *terminalEngine) SetHostTheme(
	bg color.Color,
	fg color.Color,
) {
	e.themeMu.Lock()
	defer e.themeMu.Unlock()
	if bg != nil {
		e.themeBg = bg
	}
	if fg != nil {
		e.themeFg = fg
	}
}

// hostTheme reads the recorded host colours for a session about to be born.
func (e *terminalEngine) hostTheme() (bg, fg color.Color) {
	e.themeMu.RLock()
	defer e.themeMu.RUnlock()
	return e.themeBg, e.themeFg
}

// OnSessionEnded registers the termination callback. The most recent registration wins.
func (e *terminalEngine) OnSessionEnded(
	fn func(ctx context.Context, chatID, sessionID string, exitCode int),
) {
	e.cbMu.Lock()
	e.onEnded = fn
	e.cbMu.Unlock()
}

// OnSessionState registers the state-transition callback. The most recent registration
// wins. Fires on "detached" (last-client detach or post-restore) and "suspended". Does
// not fire for "ended" — use OnSessionEnded for that.
func (e *terminalEngine) OnSessionState(
	fn func(ctx context.Context, chatID, sessionID, state string),
) {
	e.cbMu.Lock()
	e.onState = fn
	e.cbMu.Unlock()
}

// SetMetaStore injects the durable session-metadata store. The most recent call wins;
// nil removes it, and every meta helper no-ops without one.
func (e *terminalEngine) SetMetaStore(
	s SessionMetaStore,
) {
	e.cbMu.Lock()
	e.metaStore = s
	e.cbMu.Unlock()
}

func (e *terminalEngine) fireEnded(ctx context.Context, chatID, sessionID string, exitCode int) {
	e.cbMu.RLock()
	fn := e.onEnded
	e.cbMu.RUnlock()
	if fn != nil {
		fn(ctx, chatID, sessionID, exitCode)
	}
}

func (e *terminalEngine) fireState(ctx context.Context, chatID, sessionID, state string) {
	e.cbMu.RLock()
	fn := e.onState
	e.cbMu.RUnlock()
	if fn != nil {
		fn(ctx, chatID, sessionID, state)
	}
}
