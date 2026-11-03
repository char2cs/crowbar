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
	"github.com/char2cs/crowbar/api/internal/domain/problem"
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
var ErrSessionNotFound = problem.New(problem.NotFound, "terminal: session not found")

// ErrShuttingDown is returned by the session-birth paths once Shutdown has begun: the kill loop has
// already walked the registry, so a new PTY would outlive the daemon with nothing left to reap it.
var ErrShuttingDown = errors.New("terminal: engine is shutting down")

// ErrCommandNotFound is returned by CreateCommand when argv[0] is missing or not executable: a missing
// dependency, classified where exec fails so callers never string-match exec's error text.
var ErrCommandNotFound = problem.New(problem.FailedDependency, "terminal: command not found")

// Exit is how a command session's process ended: its exit code (-1 when
// unknown) and the text on its screen at the end.
type Exit struct {
	Code   int
	Screen string
}

// Engine is the full PTY session operation surface.
type Engine interface {
	// Create spawns a new PTY session owned by chatID, running in workspaceDir. chatID is the owner the
	// session is registered, listed and reaped under; workspaceDir is only where it starts, and sibling
	// chats routinely share it.
	Create(
		ctx context.Context,
		chatID string,
		workspaceDir string,
		prof *domain.TerminalProfile,
	) (sessionID string, err error)

	// CreateCommand spawns an explicit argv+env as a registered session, skipping profile resolution.
	// onExit, if non-nil, runs exactly once after the session is fully reaped (natural exit or Kill),
	// never for a merely suspended session.
	CreateCommand(
		ctx context.Context,
		chatID string,
		cwd string,
		argv []string,
		env []string,
		onExit func(Exit),
	) (sessionID string, err error)

	// Attach streams an existing session to conn: snapshot first, then live output, restoring a suspended
	// session (never an exited one). An exit frame precedes close so the client can tell a shell ending
	// from a dropped transport; every write has a deadline so a stalled client cannot pin it.
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

	// TerminateGraceful sends a running CLI SIGTERM (a PID-level action, never a PTY write) and falls
	// back to Kill after a grace window, so a well-behaved CLI can flush its transcript before a switch.
	TerminateGraceful(
		ctx context.Context,
		sessionID string,
	) error

	// Suspend tears down an idle, unattached session's PTY and persists its scrollback; Attach restores
	// it. Returns nil when the session is not eligible.
	Suspend(
		ctx context.Context,
		sessionID string,
	) error

	// SetHostTheme records the host terminal's default colours, which every session born afterwards
	// answers OSC 10/11 queries with from birth.
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

	// SessionLive reports whether a session is backed by a live PTY right now. SessionExists is also true
	// for a suspended placeholder whose process is dead, so liveness checks must use this.
	SessionLive(
		ctx context.Context,
		sessionID string,
	) bool

	// Screen renders a session's visible screen as plain text and whether it changed since generation
	// `since` (0 always returns text). It is a side-effect-free pull, unlike Attach; missing, suspended
	// or text-less sessions report ("", 0, false).
	Screen(
		sessionID string,
		since uint64,
	) (text string, gen uint64, changed bool)

	// SetMetaStore injects the durable session metadata store after construction to avoid an
	// engine → usecase import cycle; a nil store is a valid no-op.
	SetMetaStore(
		s SessionMetaStore,
	)

	// LoadPlaceholder idempotently registers a durable session from disk in the suspended state so a
	// later Attach can restore it.
	LoadPlaceholder(
		ctx context.Context,
		m SessionMeta,
		scrollback []byte,
	) error

	// Stats returns session counts, estimated model memory, and parse health (degraded sessions and
	// recovered parse panics) for observability.
	Stats() (active, detached, suspended int, modelBytes int64, degraded, parsePanics int)

	// Shutdown kills every session and blocks until each onExit callback has run: those record a dying
	// CLI's runner exit and turn close, so it must run before the databases they write are closed.
	// Afterwards session births return ErrShuttingDown. Idempotent.
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

// MaxTotalSessions returns the global ceiling on live plus suspended sessions; startup restore caps
// reloads at it so a large on-disk backlog cannot blow the in-memory ceiling.
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
	metaStore SessionMetaStore

	// themeMu guards the host theme, held on the engine because a CLI may query its background within
	// milliseconds of exec, before any session exists. Nil means unknown, and stays so.
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

// SetHostTheme records the host default colours every subsequently born session answers OSC 10/11
// with; Session.SetTheme covers existing sessions. A nil channel is a no-op for that channel.
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

// SetMetaStore injects the durable session-metadata store. The most recent call wins;
// nil removes it, and every meta helper no-ops without one.
func (e *terminalEngine) SetMetaStore(
	s SessionMetaStore,
) {
	e.cbMu.Lock()
	e.metaStore = s
	e.cbMu.Unlock()
}
