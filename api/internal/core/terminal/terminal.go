// Package terminal is the PTY session engine. It manages session lifecycle,
// serialized-snapshot redraw on (re)attach, and multi-client WebSocket fan-out.
package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/persistence"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/profile"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/registry"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
	"github.com/char2cs/crowbar/api/internal/domain"
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

	// A GUI/launchd-launched daemon inherits launchd's minimal environment, which
	// carries NO locale — so the PTY shell falls back to the POSIX "C" locale and
	// mangles every non-ASCII byte (a typed argument or program output alike) into
	// U+FFFD. Default a UTF-8 locale when the inherited environment specifies none,
	// so "works in a real terminal, broken in the packaged app" glyph corruption
	// cannot happen. Never overrides a user's own explicit locale.
	if lang := defaultLocale(base, runtime.GOOS); lang != "" {
		overrides["LANG"] = lang
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

// defaultLocale returns the UTF-8 LANG value ptyEnv should inject when the
// inherited environment carries NO locale at all, or "" when a locale is already
// present. It defaults ONLY when all three of LANG, LC_ALL and LC_CTYPE are
// unset, so a user's explicit locale is never overridden. macOS ships no
// C.UTF-8 locale, so darwin falls back to en_US.UTF-8; Linux (and CI) use the
// locale-independent C.UTF-8 — keyed off goos so the choice is portable.
func defaultLocale(
	base []string,
	goos string,
) string {
	for _, entry := range base {
		if strings.HasPrefix(entry, "LANG=") ||
			strings.HasPrefix(entry, "LC_ALL=") ||
			strings.HasPrefix(entry, "LC_CTYPE=") {
			return ""
		}
	}
	if goos == "darwin" {
		return "en_US.UTF-8"
	}
	return "C.UTF-8"
}

// DefaultLocaleForTest exposes the internal defaultLocale decision to the
// package's external unit tests so they can assert ptyEnv's per-GOOS UTF-8
// fallback for a synthetic environment without mutating the real process
// environment. It returns the LANG value ptyEnv would inject for the given base
// environment and GOOS, or "" when a locale is already set.
func DefaultLocaleForTest(
	base []string,
	goos string,
) string {
	return defaultLocale(base, goos)
}

// ParseHexColor converts the frontend's resolved CSS colour ("#rgb", "#rrggbb", or
// "#rrggbbaa" — the form resolve-css-color.ts emits) into a color.Color, or nil when the
// string is empty/unparseable. Alpha is dropped: the value feeds an OSC 11/10 default-colour
// report, which is RGB-only. A nil result is a safe no-op downstream — Session.SetTheme and
// SetHostTheme both leave a nil channel unchanged rather than resetting it.
//
// Exported because the hex string is the wire form on BOTH theme channels: the per-session
// WS frame handled below, and the host-theme REST push the API layer serves.
func ParseHexColor(s string) color.Color {
	if len(s) == 0 || s[0] != '#' {
		return nil
	}
	h := s[1:]
	if len(h) == 3 { // #rgb shorthand -> #rrggbb
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 && len(h) != 8 {
		return nil
	}
	v, err := strconv.ParseUint(h[:6], 16, 32)
	if err != nil {
		return nil
	}
	//nolint:gosec // v is a parsed 24-bit hex colour; these shifts/masks extract the R/G/B bytes and cannot overflow uint8.
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
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
//
// It exists because the packaged .app shipped a version where every agent chat died
// on exactly this and surfaced as an unmapped HTTP 500, which the UI dropped on the
// floor: the user saw a chat button that did nothing at all. A missing CLI must be
// SAYABLE.
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
	// live output. If the session is
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

	// Suspend intentionally tears down an idle, unattached session's PTY,
	// persists its scrollback to disk, and replaces the registry entry with a
	// placeholder. A subsequent Attach transparently restores it. Returns nil
	// when the session is not eligible (has clients, not idle, or not live).
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

	// OnSessionEnded registers the callback invoked when a session terminates
	// (a Kill, a Shutdown, or a PTY self-exit). It fires exactly once per
	// session so the lifecycle topic can emit an "ended" frame. exitCode is
	// the process exit code captured by shutdown(); -1 if unknown (killed by
	// signal, placeholder, or daemon restart).
	OnSessionEnded(
		fn func(ctx context.Context, chatID, sessionID string, exitCode int),
	)

	// OnSessionState registers the callback invoked when a session transitions
	// to "detached" (last client disconnects or post-restore) or "suspended"
	// (placeholder swap complete). The most recent registration wins.
	OnSessionState(
		fn func(ctx context.Context, chatID, sessionID, state string),
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

// sessionRegistry is the subset of *registry.Registry the engine uses. It is an interface
// only so a test can substitute a hooked registry that drives the engine's
// otherwise-unreachable lookup-race guards (a List id that Get cannot resolve, a Get'able
// session whose ChatID has concurrently vanished). Production always uses
// *registry.Registry.
type sessionRegistry interface {
	Add(id, chatID string, s *session.Session)
	Get(id string) (*session.Session, bool)
	Remove(id string)
	List() []string
	ListByChat(chatID string) []string
	ChatID(id string) (string, bool)
}

type terminalEngine struct {
	reg sessionRegistry

	mu         sync.RWMutex
	onEnded    func(ctx context.Context, chatID, sessionID string, exitCode int)
	onState    func(ctx context.Context, chatID, sessionID, state string)
	endedOnce  map[string]struct{}
	metaStore  SessionMetaStore
	lastActive map[string]time.Time // guarded by mu; set on last-client detach

	// sessionMu serialises per-session lifecycle operations (restore, suspend,
	// Kill) so concurrent Attach calls on a placeholder never spawn two shells
	// or leave clients attached to a dead placeholder. Lock order:
	// sessionMu (outer) → s.mu/flushMu (inner). Never reverse.
	sessionMu sync.Map // map[string]*sync.Mutex

	// cmdCleanups holds the optional onExit callback passed to CreateCommand,
	// keyed by session id. reapOnDone fires and removes it exactly once when the
	// session's PTY session ends (natural exit or Kill) — never on suspend, since
	// the caller's resources (e.g. an agentic CLI's hook-config tmp dir) must
	// outlive the running process, not just an idle-detach.
	cmdCleanups sync.Map // map[string]func()

	// themeMu guards the host theme below. It is deliberately NOT e.mu: the host theme is
	// read on the session-birth path (spawn/CreateCommand), and e.mu is held across
	// callback dispatch — sharing it would put a lock used by every PTY birth behind
	// unrelated notification work.
	themeMu sync.RWMutex
	// themeBg/themeFg are the HOST TERMINAL's default colours — the single light/dark
	// truth shared by every session in the Crowbar window, not per-session state. They are
	// held here, on the engine, because they must be known BEFORE a session exists: a
	// vendor CLI is already running by the time CreateCommand returns an id, and one that
	// detects its theme by querying the background does so within milliseconds of exec.
	//
	// Nil (never pushed) means "unknown", and unknown must stay unknown: the seed is
	// skipped and the emulator keeps its own default, so nothing here can invent a colour.
	themeBg, themeFg color.Color

	// reaps tracks the reapOnDone goroutines so Shutdown can JOIN them: every exit
	// callback the engine still owes has RUN by the time Shutdown returns. See
	// reapTracker — this is what makes "the databases outlive their writers" a
	// structural fact rather than a race the shutdown sequence happens to win.
	reaps reapTracker

	stop     chan struct{}
	stopOnce sync.Once
	// maintDone is closed by maintenanceLoop when it returns, so Shutdown can JOIN the
	// maintenance goroutine before tearing down sessions — guaranteeing no background sweep
	// (which reads the package-level limit vars) outlives Shutdown.
	maintDone chan struct{}
}

// reapTracker counts the live reapOnDone goroutines and lets Shutdown wait them
// out. It exists because a reap goroutine is a WRITER: the onExit callback it
// fires is how a dying vendor CLI's death is recorded (the agent usecase Exits
// the runner and closes the turn the CLI abandoned mid-flight). Those writes go
// to databases the shutdown sequence is on its way to closing, so the reap must
// be a step OF the shutdown, not a goroutine racing it.
//
// It is a counter + a mutex rather than a sync.WaitGroup on purpose. A WaitGroup
// PANICS ("Add called concurrently with Wait") if a session is born while
// Shutdown is waiting — the exact race a graceful stop creates, since a hijacked
// WebSocket can still drive Attach→restore after the HTTP server has stopped
// accepting. Making "admit a new reap" and "start draining" the same critical
// section removes the race instead of documenting it: once draining, start()
// refuses, so the outstanding set can only shrink and the drain always converges.
type reapTracker struct {
	mu       sync.Mutex
	n        int
	draining bool
	idle     chan struct{} // non-nil only while a drain is waiting on n > 0
}

// start admits a new reap goroutine, reporting false once a drain has begun (the
// caller must then NOT spawn the session — see ErrShuttingDown).
func (t *reapTracker) start() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.draining {
		return false
	}
	t.n++
	return true
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

// drain closes the door on new reaps and returns a channel closed once every
// outstanding one has returned. It is idempotent, and it is safe to call BEFORE
// the sessions are killed: a live session always has its reap goroutine parked on
// s.Done(), so it is already counted here — the kill is merely what lets it
// finish.
func (t *reapTracker) drain() <-chan struct{} {
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

var _ Engine = (*terminalEngine)(nil)

// Session limits — package-level so tests can override them.
var (
	softLimitPerChat   = 10               // max simultaneous detached sessions per chat
	maxTotalSessions   = 100              // global hard ceiling (count)
	maxTotalModelBytes = int64(256) << 20 // global hard ceiling (~serialized model bytes)
)

// maintenanceTickInterval is the cadence of the background maintenance sweep. It is a
// package-level var only so a test can shorten it to drive the maintenanceLoop ticker branch
// deterministically; production uses the 10-second default.
var maintenanceTickInterval = 10 * time.Second

// startupWrite issues one resolved profile startup command to a freshly-created session. It
// is a package-level var only so a test can force the write-failure branch in Create (a live
// PTY whose write fails); production always delegates straight to session.Write.
var startupWrite = func(s *session.Session, data []byte) error {
	return s.Write(data)
}

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
		maintDone:  make(chan struct{}),
	}
	go e.maintenanceLoop()
	return e
}

// SetHostTheme records the host terminal's default background/foreground colours as the
// values every SUBSEQUENTLY BORN session answers an OSC 10/11 query with.
//
// It is the "future sessions" half of theme propagation, and it is a different job from
// Session.SetTheme, which is the "sessions that already exist" half (it also emits the DEC
// 2031 CSI ?997;n report that makes a subscribed, already-running app re-query live).
// Both are needed, and neither substitutes for the other:
//
//   - Without this, a session is born answering x/vt's hardcoded black. An app that queries
//     once at startup and never subscribes to 2031 — codex 0.146.0 is exactly that — reads
//     black, picks dark, and latches it for the life of the process. The frontend's
//     attach-time push cannot fix it: the process was exec'd by CreateCommand, before any
//     client could attach, and nothing will ever ask again.
//   - Without SetTheme, a theme switch would never reach a CLI that is already running.
//
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

// hostTheme reads the recorded host colours for a session about to be born. Both are nil
// until something pushes a theme, which WithTheme treats as "leave the emulator default".
func (e *terminalEngine) hostTheme() (bg, fg color.Color) {
	e.themeMu.RLock()
	defer e.themeMu.RUnlock()
	return e.themeBg, e.themeFg
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

// outputMsg is the server→client wire frame. Snapshot marks a self-contained
// ground-state redraw the client must apply onto a RESET buffer (attach
// redraw, post-resize resync) instead of appending like incremental output.
type outputMsg struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
	Snapshot  bool   `json:"snapshot,omitempty"`
}

// inputMsg is the client→server wire frame for PTY input.
//
// The "theme" type carries the host terminal's light/dark theme so a foreground app's
// automatic theme can follow a Crowbar theme switch: Bg/Fg are the resolved default
// background/foreground colours (as "#rrggbb", the values an OSC 11/10 query answers with)
// and Dark is the authoritative light/dark polarity for the CSI ?997;n theme-change report.
type inputMsg struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
	Bg   string `json:"bg"`
	Fg   string `json:"fg"`
	Dark bool   `json:"dark"`
}

// engineBirth carries exactly one of the two birth modes to e.spawn (§9.1): create
// (Blob == nil, size+scrollback from Cols/Rows/ScrollbackLines) or restore (Blob != nil,
// the raw .buf bytes including the CRWB1 header, plus an optional daemon-authored on-screen
// Notice injected into the model BEFORE the session is registered for attach).
type engineBirth struct {
	Cols            int
	Rows            int
	ScrollbackLines int
	Blob            []byte
	Notice          []byte
}

// spawn creates a live session (create or restore), registers it in the registry under
// id/chatID, and starts reapOnDone. It is the single point of session birth — both
// Create and restore delegate here. For the restore path it injects the on-screen Notice
// into the model BEFORE e.reg.Add exposes the session, so the very first concurrent Attach
// cannot observe the registered session without the notice already applied (§12 race fix).
func (e *terminalEngine) spawn(
	id string,
	chatID string,
	shell string,
	cwd string,
	profileID string,
	b engineBirth,
) (*session.Session, error) {
	var (
		s   *session.Session
		err error
	)
	// The host theme is seeded at BIRTH, not after: session.WithTheme installs it before the
	// session's io pump goroutine starts, so the model is already answering with the truth
	// before the spawned process can get a query in. See SetHostTheme.
	bg, fg := e.hostTheme()
	if b.Blob != nil {
		s, err = session.NewRestored(id, shell, cwd, profileID, ptyEnv(), b.Blob, session.WithTheme(bg, fg))
	} else {
		s, err = session.New(id, shell, cwd, profileID, ptyEnv(), b.Cols, b.Rows, b.ScrollbackLines,
			session.WithTheme(bg, fg))
	}
	if err != nil {
		return nil, err
	}
	// Claim a reap slot BEFORE the session is registered or its reaper launched: a
	// session born after Shutdown's kill loop has walked the registry would never be
	// reaped at all, so refuse it and take the PTY we just spawned back down.
	if !e.reaps.start() {
		s.Kill()
		return nil, ErrShuttingDown
	}
	if len(b.Notice) > 0 {
		s.InjectLocal(b.Notice)
	}
	e.reg.Add(id, chatID, s)
	go e.reapOnDone(id, chatID, s)
	return s, nil
}

func (e *terminalEngine) Create(
	_ context.Context,
	chatID string,
	workspaceDir string,
	prof *domain.TerminalProfile,
) (string, error) {
	resolved := profile.Resolve(prof, workspaceDir)
	id := uuid.NewString()

	// Create births at the historical 80×24 default with the default scrollback depth; the
	// session resolves the zero values (§9.1 step 5). FE-measured dims are a deferred wire
	// addition — until then a fresh attach's first resize reshapes both PTY and model.
	s, err := e.spawn(id, chatID, resolved.Shell, resolved.CWD, "", engineBirth{})
	if err != nil {
		return "", fmt.Errorf("terminal: create: %w", err)
	}

	for _, cmd := range resolved.Startup {
		if writeErr := startupWrite(s, []byte(cmd+"\n")); writeErr != nil {
			// Non-fatal: PTY is alive but startup command failed.
			break
		}
	}

	return id, nil
}

// CreateCommand spawns an explicit argv+env as a registered session (streamable
// over the terminal WS). Used by the agentic engine for vendor-CLI segments.
// onExit, if non-nil, is recorded and fired once by reapOnDone when the session
// terminates — see the Engine interface doc for the exact contract.
func (e *terminalEngine) CreateCommand(
	_ context.Context,
	chatID string,
	cwd string,
	argv []string,
	env []string,
	onExit func(),
) (string, error) {
	id := uuid.NewString()
	// The real terminal engine seeds ptyEnv() (TERM/COLORTERM); CreateCommand takes
	// the caller's env verbatim, so under launchd TERM is absent and Ink TUIs
	// misrender. Backfill the terminal defaults for any keys not already set.
	env = withTerminalDefaults(env)
	// Seeded at birth, before the pump — this is the path every agentic vendor CLI takes,
	// and the one the whole host-theme mechanism exists for. See SetHostTheme.
	bg, fg := e.hostTheme()
	s, err := session.NewCommand(id, argv, cwd, env, 80, 24, 0, session.WithTheme(bg, fg))
	if err != nil {
		// exec.ErrNotFound means argv[0] is not installed / not executable — a fact about
		// the USER'S MACHINE, not a server fault. Classify it into ErrCommandNotFound so
		// the caller can say so out loud instead of emitting an opaque 500.
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("%w: %s", ErrCommandNotFound, argv[0])
		}
		return "", fmt.Errorf("terminal: create command: %w", err)
	}
	// Claim a reap slot first (see spawn): a vendor CLI whose reaper never runs is
	// strictly worse than one that is never spawned — its onExit is the ONLY thing
	// that records the runner's death and closes the turn it was mid-way through.
	if !e.reaps.start() {
		s.Kill()
		return "", ErrShuttingDown
	}
	// Store BEFORE launching the reap goroutine so it can never observe a
	// self-exit and look up the cleanup before it is recorded.
	if onExit != nil {
		e.cmdCleanups.Store(id, onExit)
	}
	e.reg.Add(id, chatID, s)
	// Deliberately NOT the request context: the reaper has to outlive the call that
	// created the session — a PTY is reaped when the process exits, which is minutes
	// or hours after the HTTP request returned. It is joined by e.reaps, which
	// Shutdown drains.
	go e.reapOnDone(id, chatID, s) //nolint:gosec // G118: detached by design; joined by e.reaps, not by the caller's ctx.
	return id, nil
}

// withTerminalDefaults appends TERM=xterm-256color / COLORTERM=truecolor and a
// UTF-8 LANG only for keys the caller did not already provide, matching the real
// terminal engine's ptyEnv() seeding.
func withTerminalDefaults(env []string) []string {
	has := func(key string) bool {
		for _, kv := range env {
			if strings.HasPrefix(kv, key+"=") {
				return true
			}
		}
		return false
	}
	if !has("TERM") {
		env = append(env, "TERM=xterm-256color")
	}
	if !has("COLORTERM") {
		env = append(env, "COLORTERM=truecolor")
	}
	// The locale half of the same launchd-minimal-environment hazard ptyEnv() guards.
	// CreateCommand callers pass os.Environ() straight through (see the agent usecase),
	// so a GUI-launched daemon hands the vendor CLI an environment with NO locale at
	// all — and this backfill used to stop at TERM, which is why the interactive
	// terminal was fixed and agent chats were not.
	//
	// With no locale, CoreFoundation falls back to __CF_USER_TEXT_ENCODING, whose
	// script code is 0 (Mac OS Roman) on a default macOS account. Claude Code copies
	// via BOTH OSC 52 and a spawned pbcopy; we drop OSC 52 (finishOSC routes only
	// codes 0/1/2, and the frontend parses only OSC 7), so pbcopy's value is what
	// lands — and it reads the CLI's UTF-8 bytes as Mac Roman. "—" reaches the
	// pasteboard as "‚Äî" while the SCREEN stays correct, because the render path
	// never touches pbcopy. Every copy applies exactly one more round of it.
	//
	// defaultLocale returns "" the moment the caller set any of LANG/LC_ALL/LC_CTYPE,
	// so an explicit locale is never overridden.
	if lang := defaultLocale(env, runtime.GOOS); lang != "" {
		env = append(env, "LANG="+lang)
	}
	return env
}

// reapOnDone removes the session from the registry once it terminates and fires
// the OnSessionEnded callback so the lifecycle topic can emit an "ended" frame.
// It covers every real-termination path: an explicit Kill, a Shutdown, or a PTY
// self-exit. For the suspend path it returns early so it does NOT remove the
// placeholder or fire ended.
func (e *terminalEngine) reapOnDone(
	id string,
	chatID string,
	s *session.Session,
) {
	// Registered FIRST so it runs LAST (defers are LIFO): even a panic recovered by
	// safego below must retire this reap, or Shutdown's drain would wait on a
	// goroutine that is already gone.
	defer e.reaps.done()
	defer safego.Recover("terminal.reapOnDone")

	// Wait for termination BEFORE acquiring the per-session lock so we never
	// block other lifecycle ops (Kill/suspend/flush/detach) while parked here.
	<-s.Done()

	// FIX: serialise the real-exit cleanup with the cadence-flush and
	// detach-bookkeeping persist paths via the per-session lifecycle lock. Lock
	// order: sessionMu (outer) → s.mu/flushMu — we hold only sessionMu here.
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

	// Fire the CreateCommand caller's onExit exactly once, now that the session
	// is fully reaped: the running CLI (if any) that read the callback's backing
	// files for its whole lifetime is guaranteed gone.
	if v, ok := e.cmdCleanups.LoadAndDelete(id); ok && v != nil {
		v.(func())()
	}

	ctx := context.Background()
	dir, _ := e.storageDir(ctx, chatID)
	if dir != "" {
		if delErr := persistence.DeleteBuf(dir, id); delErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "terminal: reap: delete buf %s: %v\n", id, delErr)
		}
	}
	e.deleteMeta(ctx, id)
	e.fireEnded(ctx, chatID, id, exitCode)

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
	chatID string,
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

	fn(ctx, chatID, sessionID, exitCode)
}

// fireState invokes the registered OnSessionState callback if one is set.
// It is nil-safe: if no callback has been registered the call is a no-op.
func (e *terminalEngine) fireState(
	ctx context.Context,
	chatID string,
	sessionID string,
	state string,
) {
	e.mu.RLock()
	fn := e.onState
	e.mu.RUnlock()
	if fn == nil {
		return
	}
	fn(ctx, chatID, sessionID, state)
}

// OnSessionEnded registers the termination callback. The most recent
// registration wins, mirroring the LSP engine's OnDiagnostics hook.
func (e *terminalEngine) OnSessionEnded(
	fn func(ctx context.Context, chatID, sessionID string, exitCode int),
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
	fn func(ctx context.Context, chatID, sessionID, state string),
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
	e.reg.Add(m.SessionID, m.ChatID, ph)
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

// storageDir resolves the per-chat storage directory via the injected
// store. It returns ("", nil) when metaStore is nil.
func (e *terminalEngine) storageDir(
	ctx context.Context,
	chatID string,
) (string, error) {
	e.mu.RLock()
	ms := e.metaStore
	e.mu.RUnlock()
	if ms == nil {
		return "", nil
	}
	return ms.StorageDir(ctx, chatID)
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

	chatID, chatOK := e.reg.ChatID(sid)
	if !chatOK {
		unlock()
		return fmt.Errorf("terminal: restore: chat not found for session %s", sid)
	}

	shell := s.Shell()
	cwd := s.CWD()
	profileID := s.ProfileID()

	var rawBlob []byte
	dir, dirErr := e.storageDir(ctx, chatID)
	if dirErr == nil && dir != "" {
		rawBlob, _ = persistence.ReadBuf(dir, sid)
	}

	// CWD fallback: the saved working directory may have been deleted (worktree
	// removed, repo cleaned) since suspend. Spawning with a non-existent cmd.Dir
	// fails, which — left unhandled — would strand the placeholder and make every
	// future Attach retry the same doomed spawn forever. Stat first and fall back
	// to the user's home directory; the substitution notice is injected into the
	// model (never the persisted .buf) via engineBirth.Notice (§12).
	cwd, notice := resolveRestoreCWD(cwd)

	if _, err := e.spawn(sid, chatID, shell, cwd, profileID, engineBirth{Blob: rawBlob, Notice: notice}); err != nil {
		// A refusal because the ENGINE IS SHUTTING DOWN says nothing about the
		// session: it is perfectly restorable, we are simply not birthing anything
		// any more (the kill loop has already walked the registry, so a session born
		// now would have no reaper). Dropping it here would delete a healthy user's
		// scrollback and meta row on the way out of the daemon — destroying the very
		// state the graceful shutdown just persisted for the next boot to restore.
		// Leave the placeholder exactly as it is and fail the Attach; the next daemon
		// start reloads it. (A hijacked WebSocket can still drive Attach after the
		// HTTP server has stopped accepting, so this is reachable.)
		if errors.Is(err, ErrShuttingDown) {
			unlock()
			return fmt.Errorf("terminal: restore: spawn: %w", err)
		}
		// Un-restorable even after the CWD fallback (e.g. the shell binary itself
		// is gone). Never leave the placeholder in the registry: drop it, delete
		// its persisted state, and fire ended so the FE removes the dead tab
		// instead of looping on Attach.
		//
		// Release the lifecycle lock before dropUnrestorable and the subsequent
		// sessionMu.Delete — matching reapOnDone's and Kill's unlock-before-delete
		// ordering so the entry is pruned while no lock is held.
		unlock()
		e.dropUnrestorable(ctx, chatID, sid, dir)
		e.sessionMu.Delete(sid)
		return fmt.Errorf("terminal: restore: spawn: %w", err)
	}

	// Record restored session as detached (live but no clients yet).
	e.saveMeta(ctx, SessionMeta{
		SessionID: sid,
		ChatID:    chatID,
		CWD:       cwd,
		Shell:     shell,
		ProfileID: profileID,
		State:     "detached",
	})
	e.fireState(ctx, chatID, sid, "detached")

	unlock()
	return nil
}

// resolveRestoreCWD returns a working directory guaranteed to exist for a restore spawn,
// plus an optional on-screen notice. If the saved cwd still resolves to a directory it is
// returned with no notice; otherwise it falls back to the user's home directory (or "" so
// the shell defaults) and returns a one-line notice the engine injects into the model (never
// the persisted blob) so the user sees why their previous directory was not used.
func resolveRestoreCWD(
	cwd string,
) (string, []byte) {
	if dirExists(cwd) {
		return cwd, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	notice := fmt.Sprintf(
		"\r\n[crowbar] previous directory unavailable; restored in %s\r\n", home,
	)
	return home, []byte(notice)
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
	chatID string,
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
	e.fireEnded(ctx, chatID, sid, -1)

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

	chatID, chatOK := e.reg.ChatID(sid)
	if !chatOK {
		return nil
	}

	now := time.Now()

	// Step a: capture state + persist BEFORE replacing registry or killing.
	// FIX 2: hold flushMu across snapshot+write so this path serialises with
	// the cadence-flush, detach-bookkeeping, and shutdown flush paths.
	dir, _ := e.storageDir(ctx, chatID)
	fm := s.FlushMu()
	fm.Lock()
	var blob []byte
	if force && wasRunning {
		// Force-suspend of a LIVE app: fuse teardown + notice + serialize in ONE s.mu hold
		// inside the session so no live chunk can re-alt the model between them (§11.2).
		// Persist the returned clean-primary blob verbatim — no second Snapshot.
		blob = s.ForceSuspendSnapshot(
			[]byte("\r\n[crowbar] session suspended to free resources; re-open to restore\r\n"),
		)
	} else {
		blob, _ = s.Snapshot()
	}
	if dir != "" {
		if err := persistence.WriteBuf(dir, sid, blob); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "terminal: suspend: persist buf %s: %v\n", sid, err)
		}
	}
	fm.Unlock()
	e.saveMeta(ctx, SessionMeta{
		SessionID:    sid,
		ChatID:       chatID,
		CWD:          s.CWD(),
		Shell:        s.Shell(),
		ProfileID:    s.ProfileID(),
		State:        "suspended",
		LastActiveAt: now,
	})

	// Step b: replace registry entry with placeholder BEFORE killing.
	ph := session.NewPlaceholder(sid, s.Shell(), s.CWD(), s.ProfileID(), blob)
	e.reg.Add(sid, chatID, ph)

	// Step c: kill the OLD live session.
	s.Kill()

	// Notify lifecycle subscribers that the session is now suspended.
	e.fireState(ctx, chatID, sid, "suspended")

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
	if !s.IsLive() { //nolint:nestif // restore-then-refetch-then-reattach ladder; the post-restore re-fetch guards are load-bearing
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

// persistableSession reports whether a session may be written to durable
// storage (.buf + meta row) — i.e. whether restoring it on a later boot is even
// meaningful.
//
// A COMMAND session (session.NewCommand: an agentic vendor CLI spawned with an
// explicit argv) is NOT persistable, and writing a row for one is actively
// harmful. Restore only knows how to birth a LOGIN SHELL: it re-spawns
// s.Shell(), which for a command session is the joined argv string — a bogus
// binary. So a persisted command session comes back from RestorePersistedSessions
// as a PTY-less placeholder that either (a) resurrects as a BARE SHELL in the
// agent's chat pane, or (b) fails its restore-spawn outright — while the agent
// read model, whose boot reconcile sees a registered id and believes the CLI
// survived, keeps advertising a live agent that is long dead.
//
// The suspend paths already refuse command sessions
// (session.BeginSuspendIfEligible / BeginForceSuspend) and the maintenance sweep
// skips them; this is the same invariant for the three remaining durable-write
// paths (cadence flush, detach bookkeeping, graceful Shutdown). A command
// session's process dies with the daemon, by design — the agent usecase's boot
// reconcile is what ends its segment, and it can only do that if the terminal
// engine does not pretend the session came back.
func persistableSession(s *session.Session) bool {
	return !s.IsCommand()
}

// persistOnDetach writes the last-known scrollback + "detached" meta when the
// final client leaves. It takes the per-session lifecycle lock and checks
// s.IsLive() first: if the session died (self-exit/Kill) while we were
// detaching, reapOnDone owns cleanup and persisting here would resurrect an
// explicitly-closed terminal — so we skip. Lock order: sessionMu (outer) →
// s.mu/flushMu.
func (e *terminalEngine) persistOnDetach(ctx context.Context, sessionID string, s *session.Session) {
	unlock := e.lockSession(sessionID)
	defer unlock()

	// Lost the race to reap (or to a suspend that swapped in a placeholder):
	// the reap/suspend path owns the .buf/row — do not write.
	if !s.IsLive() {
		return
	}
	// A command session (agentic vendor CLI) is UNRESTORABLE — see
	// persistableSession. Detaching the agent pane must not leave a durable row
	// behind for the next boot to resurrect.
	if !persistableSession(s) {
		return
	}
	chatID, chatOK := e.reg.ChatID(sessionID)
	if !chatOK {
		return
	}

	now := time.Now()
	e.mu.Lock()
	e.lastActive[sessionID] = now
	e.mu.Unlock()

	dir, _ := e.storageDir(ctx, chatID)
	if dir != "" {
		// Hold flushMu across snapshot+write so concurrent flush paths
		// (cadence, suspend, shutdown) don't interleave writes.
		fm := s.FlushMu()
		fm.Lock()
		snap, _ := s.Snapshot()
		writeErr := persistence.WriteBuf(dir, sessionID, snap)
		fm.Unlock()
		if writeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "terminal: detach: persist buf %s: %v\n", sessionID, writeErr)
		}
	}
	e.saveMeta(ctx, SessionMeta{
		SessionID:    sessionID,
		ChatID:       chatID,
		CWD:          s.CWD(),
		Shell:        s.Shell(),
		ProfileID:    s.ProfileID(),
		State:        "detached",
		LastActiveAt: now,
	})
	e.fireState(ctx, chatID, sessionID, "detached")
}

// maxCoalesceBytes caps how much queued PTY output writePump merges into a
// single WebSocket message. Large enough to collapse bursts (build logs, cat,
// full-screen TUI redraws) into a handful of messages, small enough to bound
// per-message memory and keep the renderer's per-frame work reasonable.
const maxCoalesceBytes = 256 * 1024

// trailingIncompleteUTF8 returns the number of bytes at the end of b that form a
// truncated (not-yet-complete) multi-byte UTF-8 sequence — a rune whose lead byte
// has arrived but whose continuation bytes have not. Returns 0 when b ends on a
// rune boundary (empty, or ending in an ASCII byte or a complete sequence).
//
// writePump uses this to avoid splitting a multi-byte rune across two messages:
// the Data field is JSON-string-encoded, and json.Marshal replaces ANY invalid
// UTF-8 in a string with U+FFFD. A box-drawing/powerline/emoji glyph (common in
// TUIs like Claude Code) whose 2-4 bytes straddle a PTY-read / coalescing
// boundary would otherwise be corrupted to the "�"/"???" glyph on screen.
func trailingIncompleteUTF8(b []byte) int {
	// A truncated sequence is at most 3 bytes (a 4-byte rune missing up to 3).
	maxScan := 3
	if len(b) < maxScan {
		maxScan = len(b)
	}
	for i := 1; i <= maxScan; i++ {
		c := b[len(b)-i]
		if c < 0x80 {
			return 0 // ASCII byte: the tail is already on a rune boundary
		}
		if c >= 0xC0 { // a lead byte
			need := 2
			switch {
			case c >= 0xF0:
				need = 4
			case c >= 0xE0:
				need = 3
			}
			if i < need {
				return i // lead byte present but continuation bytes missing
			}
			return 0 // full sequence present
		}
		// 0x80..0xBF: continuation byte — keep scanning back for the lead byte
	}
	return 0 // no lead byte within the last 3 bytes (malformed) — leave as-is
}

// writePump reads from ch and forwards output frames to the WebSocket.
// When the write fails or ch closes, it closes the connection so readPump unblocks.
//
// To keep high-throughput output from generating one JSON WebSocket message per
// 4 KB PTY read (which floods the transport, the Tauri reader's IPC channel, and
// the renderer), each iteration opportunistically drains any frames ALREADY
// queued on ch — without blocking — and concatenates them into one message.
// Because it never waits for more data, single-keystroke echo latency is
// unchanged; only already-backed-up bursts coalesce. Each frame's Data is a
// freshly-allocated slice (session.readLoop copies every read), so appending is
// safe and never mutates a frame still sitting in the channel.
//
// A trailing incomplete UTF-8 rune is held back (via pending) and prepended to
// the next message so json.Marshal never corrupts a split multi-byte glyph to
// U+FFFD — see trailingIncompleteUTF8.
//
//nolint:gocyclo // cohesive coalescing/UTF-8-holdback state machine; splitting it would obscure the single-drain-loop invariant.
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

	// writeMsg marshals and sends one wire frame; false means the socket died.
	writeMsg := func(data []byte, snapshot bool) bool {
		msg := outputMsg{
			SessionID: sessionID,
			Data:      string(data),
			Snapshot:  snapshot,
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			return true
		}
		return conn.WriteMessage(websocket.TextMessage, payload) == nil
	}

	var pending []byte
	for frame := range ch {
		// Snapshot frames are coalescing BARRIERS: a snapshot is a
		// self-contained redraw the client applies onto a reset buffer, so it
		// must never be merged into (or split across) incremental output
		// messages. Any held-back partial rune belongs to the pre-snapshot
		// stream the reset supersedes — drop it.
		if frame.Snapshot {
			pending = pending[:0]
			if !writeMsg(frame.Data, true) {
				return
			}
			continue
		}

		buf := make([]byte, 0, len(pending)+len(frame.Data))
		buf = append(buf, pending...)
		buf = append(buf, frame.Data...)
		pending = pending[:0]
		closed := false
		var snapshotAfter *session.OutputFrame

	drain:
		for len(buf) < maxCoalesceBytes {
			select {
			case next, ok := <-ch:
				if !ok {
					closed = true
					break drain
				}
				if next.Snapshot {
					snap := next
					snapshotAfter = &snap
					break drain
				}
				buf = append(buf, next.Data...)
			default:
				break drain
			}
		}

		// Hold back a trailing incomplete rune until its remaining bytes arrive.
		// On channel close there is no more data, so flush everything as-is; a
		// queued snapshot supersedes the tail too, so no holdback either.
		if !closed && snapshotAfter == nil {
			if n := trailingIncompleteUTF8(buf); n > 0 {
				pending = append(pending, buf[len(buf)-n:]...)
				buf = buf[:len(buf)-n]
			}
		}

		if len(buf) > 0 {
			if !writeMsg(buf, false) {
				return
			}
		}
		if snapshotAfter != nil {
			if !writeMsg(snapshotAfter.Data, true) {
				return
			}
		}
		if closed {
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
		if msg.Type == "resync" {
			// Post-resize convergence: re-emit the model snapshot to attached
			// clients (no-op at an idle shell prompt — see Session.Resync).
			_ = s.Resync()
			continue
		}
		if msg.Type == "theme" {
			// Host light/dark theme changed: update the model's OSC 10/11 query
			// answers and, if the foreground app subscribed to DEC 2031, push a
			// live CSI ?997;n theme-change report (see Session.SetTheme).
			themeBg, themeFg := ParseHexColor(msg.Bg), ParseHexColor(msg.Fg)
			// Every session in a Crowbar window renders under ONE theme, so a push at any
			// of them is a statement about the host. Recording it here keeps the birth seed
			// correct even when the dedicated host-theme push never arrives — after a daemon
			// restart, say, where the frontend re-attaches its terminals but does not reboot.
			e.SetHostTheme(themeBg, themeFg)
			s.SetTheme(themeBg, themeFg, msg.Dark)
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

// gracefulTerminateGrace bounds how long TerminateGraceful waits for a
// SIGTERM'd child to exit on its own (spec §8) before falling back to a hard
// SIGKILL. ~3s is enough for a well-behaved CLI to flush and exit without
// making a provider switch feel stuck on a wedged/signal-ignoring process.
// It is a package-level var ONLY so a unit test can shorten it (via
// SetGracefulTerminateGraceForTest) to exercise the fallback-to-hard-kill
// path without a multi-second sleep. Production never reassigns it.
var gracefulTerminateGrace = 3 * time.Second

func (e *terminalEngine) Kill(
	ctx context.Context,
	sessionID string,
) error {
	return e.terminateSession(ctx, "kill", sessionID, func(s *session.Session) { s.Kill() })
}

// TerminateGraceful gracefully quits a running vendor CLI (spec §8): it sends
// a clean-exit SIGTERM (a PID-level action, never a PTY write) and waits up
// to gracefulTerminateGrace for the process to exit on its own before falling
// back to a hard Kill. Used by provider switches so a well-behaved CLI (e.g.
// Claude Code) gets the chance to flush its native transcript on a clean exit
// — a hard SIGKILL can lose the outgoing CLI's last pre-switch turn.
func (e *terminalEngine) TerminateGraceful(
	ctx context.Context,
	sessionID string,
) error {
	return e.terminateSession(ctx, "terminate", sessionID, func(s *session.Session) {
		s.Terminate(gracefulTerminateGrace)
	})
}

// terminateSession is the shared Kill/TerminateGraceful orchestration: it
// serialises with concurrent suspend/restore, removes the session from the
// registry, invokes the given per-session teardown (hard kill or graceful
// terminate), and — for placeholder sessions only, which have no reapOnDone
// goroutine listening on s.Done() — performs the reap-equivalent cleanup
// (deleteMeta/DeleteBuf/fireEnded) inline. op names the caller in wrapped
// error/log messages ("kill" or "terminate").
func (e *terminalEngine) terminateSession(
	ctx context.Context,
	op string,
	sessionID string,
	terminate func(*session.Session),
) error {
	// FIX 1: serialise with concurrent suspend / restore so this never races a
	// suspend that could resurrect the session in the registry after removal.
	unlock := e.lockSession(sessionID)

	s, ok := e.reg.Get(sessionID)
	if !ok {
		unlock()
		return fmt.Errorf("terminal: %s: %w: %s", op, registry.ErrSessionNotFound, sessionID)
	}

	// Capture whether this is a placeholder BEFORE modifying state. A placeholder
	// session has no live PTY (IsLive() == false) so no reapOnDone goroutine is
	// running to fire ended/cleanup after this returns.
	isPlaceholder := !s.IsLive()
	chatID, chatOK := e.reg.ChatID(sessionID)

	// Remove from registry eagerly so callers see it gone immediately after this returns.
	// For live sessions, reapOnDone will also call reg.Remove (idempotent no-op) and then
	// handle the remaining cleanup: deleteMeta, DeleteBuf, fireEnded.
	e.reg.Remove(sessionID)
	terminate(s)

	// For placeholder sessions (suspended state), no reapOnDone goroutine is listening
	// on s.Done(). Perform the cleanup and fire the ended callback inline.
	if isPlaceholder && chatOK { //nolint:nestif // inline teardown for a PTY-less placeholder (no reapOnDone runs); the buf/meta/map cleanup ordering is load-bearing
		exitCode := s.ExitCode() // -1 for placeholders (no process)
		dir, _ := e.storageDir(ctx, chatID)
		if dir != "" {
			if delErr := persistence.DeleteBuf(dir, sessionID); delErr != nil {
				_, _ = fmt.Fprintf(os.Stderr, "terminal: %s: delete buf %s: %v\n", op, sessionID, delErr)
			}
		}
		e.deleteMeta(ctx, sessionID)
		e.fireEnded(ctx, chatID, sessionID, exitCode)

		// FIX 4: prune per-session maps. For live sessions, reapOnDone handles
		// this after the PTY exits. For placeholders, this is the terminal path.
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

// ListSessionsForChat returns the active session IDs owned by chatID.
func (e *terminalEngine) ListSessionsForChat(
	chatID string,
) []string {
	return e.reg.ListByChat(chatID)
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

// SessionLive reports whether the session id is backed by a live PTY (see the
// Engine interface doc): registered AND holding a process, so a suspended
// placeholder — registered, but its process long dead — correctly reads false.
func (e *terminalEngine) SessionLive(
	_ context.Context,
	sessionID string,
) bool {
	s, ok := e.reg.Get(sessionID)
	return ok && s.IsLive()
}

// Screen implements the Engine's pull-style screen read: registry lookup, then the
// session's own guarded render. An unknown id is not an error here — sessions die
// under their observers, and a caller polling one is expected to simply stop getting
// answers.
func (e *terminalEngine) Screen(
	sessionID string,
	since uint64,
) (text string, gen uint64, changed bool) {
	s, ok := e.reg.Get(sessionID)
	if !ok {
		return "", 0, false
	}
	return s.ScreenText(since)
}

// Stats returns a point-in-time snapshot of session counts, estimated model memory,
// and the parse-health surface (§9.4) across all registered sessions: degraded is the
// count of currently-degraded live sessions and parsePanics the aggregate of every
// session's recovered parse panics, sourced through the optional model.ModelHealth assert
// plus the session-level backstop counter so a blanked-and-reparsed session is observable.
func (e *terminalEngine) Stats() (active, detached, suspended int, modelBytes int64, degraded, parsePanics int) {
	for _, id := range e.reg.List() {
		s, ok := e.reg.Get(id)
		if !ok {
			continue
		}
		// Count every session's estimated model bytes — a live session's grid +
		// retained scrollback + cached blob, a placeholder its persisted blob — so the
		// total reflects real resident memory across mixed session types (§9.4).
		modelBytes += s.ModelBytes()
		dg, pp := s.Health()
		if dg {
			degraded++
		}
		parsePanics += pp
		switch s.State() {
		case "active":
			active++
		case "detached":
			detached++
		case "suspended":
			suspended++
		}
	}
	return active, detached, suspended, modelBytes, degraded, parsePanics
}

// getLastActive returns the stored last-active time for a session, or the zero
// time if the session has never had a client detach recorded.
func (e *terminalEngine) getLastActive(id string) time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastActive[id]
}

// allChatIDs returns the distinct chat IDs present in the registry.
func (e *terminalEngine) allChatIDs() []string {
	all := e.reg.List()
	seen := make(map[string]struct{}, len(all))
	var out []string
	for _, id := range all {
		chatID, ok := e.reg.ChatID(id)
		if !ok {
			continue
		}
		if _, dup := seen[chatID]; !dup {
			seen[chatID] = struct{}{}
			out = append(out, chatID)
		}
	}
	return out
}

// maintenanceLoop runs runMaintenanceOnce on a 10-second ticker until the stop
// channel is closed by Shutdown.
func (e *terminalEngine) maintenanceLoop() {
	defer safego.Recover("terminal.maintenanceLoop")
	defer close(e.maintDone)
	ticker := time.NewTicker(maintenanceTickInterval)
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

// flushSessionOnce persists one session's dirty serialized model state under the
// per-session lifecycle lock. Acquiring e.lockSession(id) serialises this write
// with reapOnDone (delete) and the detach-bookkeeping persist, and the
// !s.IsLive() guard makes a flush that loses the race to reap a no-op — so a
// cadence flush can never resurrect the .buf/row of an explicitly-closed
// terminal. Lock order: sessionMu (outer) → s.mu/flushMu.
func (e *terminalEngine) flushSessionOnce(ctx context.Context, id string) {
	unlock := e.lockSession(id)
	defer unlock()

	s, ok := e.reg.Get(id)
	if !ok || !s.IsLive() {
		// Dead or gone: reapOnDone owns cleanup; never re-write its deleted .buf.
		return
	}
	// A command session (agentic vendor CLI) is UNRESTORABLE — see
	// persistableSession. The cadence flush must not mint a durable row for it.
	if !persistableSession(s) {
		return
	}
	chatID, chatOK := e.reg.ChatID(id)
	if !chatOK {
		return
	}
	dir, err := e.storageDir(ctx, chatID)
	if err != nil || dir == "" {
		return
	}
	// Hold flushMu across snapshot+write so concurrent flush paths
	// (detach-bookkeeping, suspend, shutdown) don't interleave. Snapshot consumes the
	// dirty bit and reports whether the state changed; an idle (unchanged) session skips
	// the disk write entirely, matching today's dirty gate (§8.4).
	fm := s.FlushMu()
	fm.Lock()
	snap, changed := s.Snapshot()
	if !changed {
		fm.Unlock()
		return
	}
	writeErr := persistence.WriteBuf(dir, id, snap)
	fm.Unlock()
	if writeErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "terminal: maintenance: flush buf %s: %v\n", id, writeErr)
		return
	}
	e.saveMeta(ctx, SessionMeta{
		SessionID:    id,
		ChatID:       chatID,
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
//     the serialized model state and save updated meta.
//
//  2. Per-chat soft limit — if a chat has more than
//     softLimitPerChat detached sessions, idle-gated-suspend the oldest
//     idle detached ones until back at/under the limit.
//
//  3. Global ceiling — if over maxTotalSessions or maxTotalModelBytes:
//     a. first idle-gated-suspend oldest idle detached sessions;
//     b. as a last resort, force-suspend oldest detached running sessions.
//
// This method is called on the ticker and exposed to tests via
// RunMaintenanceOnceForTest so tests can drive it directly without waiting 10 s.
//
//nolint:gocyclo,funlen // deliberately linear three-phase sweep (flush → soft limit → global ceiling); the phases share the registry snapshot and read top-to-bottom, so splitting them would only scatter the ordering contract.
func (e *terminalEngine) runMaintenanceOnce(ctx context.Context) {
	// -------------------------------------------------------------------------
	// Phase 1: cadence flush — acquire/release the per-session lock per id so
	// the sweep never holds it across the whole registry iteration.
	// -------------------------------------------------------------------------
	for _, id := range e.reg.List() {
		e.flushSessionOnce(ctx, id)
	}

	// -------------------------------------------------------------------------
	// Phase 2: per-chat soft limit (idle-gated only)
	// -------------------------------------------------------------------------
	for _, chatID := range e.allChatIDs() {
		chatSessionIDs := e.reg.ListByChat(chatID)

		var totalDetached int
		var idleCandidates []sessionCandidate
		for _, id := range chatSessionIDs {
			s, ok := e.reg.Get(id)
			if !ok || !s.IsLive() {
				continue
			}
			if s.IsCommand() {
				continue // agentic vendor CLI — never suspend/evict, never counted
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

		if totalDetached <= softLimitPerChat {
			continue
		}

		// Sort oldest-first and suspend the excess.
		sort.Slice(idleCandidates, func(i, j int) bool {
			return idleCandidates[i].lastActive.Before(idleCandidates[j].lastActive)
		})
		excess := totalDetached - softLimitPerChat
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
		if s.IsCommand() {
			continue // agentic vendor CLI — never suspend/evict, never counted
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
	// placeholder alike — and sums each one's resident model bytes, so an
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
			bytes += s.ModelBytes()
		}
		return count <= maxTotalSessions && bytes <= maxTotalModelBytes
	}

	if underCeiling() {
		return
	}

	// Phase 3 pre-step: reclaim reclaimable cached blobs before suspending or evicting any
	// session (§9.4). Because ModelBytes() counts each live session's cached blob, the
	// ceiling can be tripped purely by reclaimable cache; dropping it first (LRU-coldest)
	// avoids spurious suspend/evict churn. Returns the instant we are back under.
	live := e.liveCacheCandidates()
	sort.Slice(live, func(i, j int) bool {
		return live[i].lastActive.Before(live[j].lastActive)
	})
	for _, c := range live {
		if underCeiling() {
			return
		}
		if s, ok := e.reg.Get(c.id); ok {
			s.DropCachedBlob()
		}
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
	// only converts a live session to a placeholder (its serialized snapshot on
	// disk); it never lowers the
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

// liveCacheCandidates returns the live sessions paired with their last-active time, for the
// §9.4 Phase-3 pre-step cache reclaim ordering. DropCachedBlob is a no-op on sessions whose
// cache is already nil, so the list need not pre-filter on cache presence.
func (e *terminalEngine) liveCacheCandidates() []sessionCandidate {
	var out []sessionCandidate
	for _, id := range e.reg.List() {
		s, ok := e.reg.Get(id)
		if !ok || !s.IsLive() {
			continue
		}
		out = append(out, sessionCandidate{id, e.getLastActive(id)})
	}
	return out
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
// s.mu/flushMu — we hold only sessionMu here.
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
	chatID, _ := e.reg.ChatID(id)
	e.reg.Remove(id)

	dir, _ := e.storageDir(ctx, chatID)
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
//   - For each LIVE session, before killing, Shutdown flushes the serialized
//     screen-model snapshot to disk (persistence.WriteBuf) and saves meta with
//     state="suspended" so a subsequent daemon start can call
//     RestorePersistedSessions and hand the session back as a placeholder.
//   - s.MarkSuspendingForShutdown() is called before s.Kill() so that
//     reapOnDone takes its early-return path and does NOT delete the .buf file
//     or meta row — the delete-on-reap path is reserved for explicit Kill()
//     calls during normal operation, not daemon shutdown.
//   - Placeholder sessions (already suspended, no live PTY) are killed without
//     any additional flush because their scrollback is already on disk.
//   - COMMAND sessions (agentic vendor CLIs) are killed WITHOUT any flush, meta
//     row, or suspend mark: they are unrestorable (see persistableSession), so
//     persisting one would hand the next boot a placeholder that resurrects as a
//     bare shell and fools the agent boot reconcile into believing a dead CLI is
//     still alive. Their process is meant to die with the daemon; the agent
//     usecase's ReconcileOnBoot ends the orphaned segment on the next start.
//   - All persist operations are best-effort: errors are logged but never cause
//     Shutdown to panic or hang.
func (e *terminalEngine) Shutdown() {
	e.stopOnce.Do(func() { close(e.stop) })
	// JOIN the maintenance goroutine before tearing down sessions, so no in-flight sweep
	// (which reads the package-level limit vars and walks the registry) can run concurrently
	// with Shutdown's own teardown or with a later caller that mutates those vars.
	<-e.maintDone
	// Close the door on new sessions BEFORE the kill loop, and take the handle we
	// will join the reapers on afterwards. Doing it here, not after the loop, is what
	// makes the drain converge: a session born mid-loop would otherwise be missed by
	// the walk of the registry and leave a PTY behind with no reaper.
	//
	// Every live session already HAS its reaper parked on s.Done(), so they are all
	// counted by now; the kill below is simply what releases them.
	reaped := e.reaps.drain()
	ctx := context.Background()
	for _, id := range e.reg.List() {
		s, ok := e.reg.Get(id)
		if !ok {
			continue
		}
		// Acquire the per-session lifecycle lock so that a session that
		// self-exits at the exact shutdown instant cannot race reapOnDone's
		// DeleteBuf+deleteMeta against Shutdown's WriteBuf+saveMeta. Lock is
		// released before the next iteration — we never hold it across the
		// whole loop. Each inner operation is non-blocking so there is no risk
		// of deadlock: reapOnDone waits on <-s.Done() BEFORE acquiring the
		// lock, and the session's Done channel is closed only by s.Kill() which
		// Shutdown calls AFTER releasing this lock.
		unlock := e.lockSession(id)
		if s.IsLive() && persistableSession(s) { //nolint:nestif // graceful-shutdown flush+meta+mark sequence per live session; the ordering (persist before Kill) is the restart-restore contract
			chatID, chatOK := e.reg.ChatID(id)
			if chatOK {
				// a) Flush scrollback to disk (best-effort; continue on error).
				// FIX 2: hold flushMu across snapshot+write to serialise with
				// any concurrent cadence-flush or detach-bookkeeping flush.
				dir, _ := e.storageDir(ctx, chatID)
				if dir != "" {
					fm := s.FlushMu()
					fm.Lock()
					snap, _ := s.Snapshot()
					shutErr := persistence.WriteBuf(dir, id, snap)
					fm.Unlock()
					if shutErr != nil {
						_, _ = fmt.Fprintf(os.Stderr, "terminal: shutdown: persist buf %s: %v\n", id, shutErr)
					}
				}
				// b) Persist meta with state="suspended" for restart restore.
				e.saveMeta(ctx, SessionMeta{
					SessionID:    id,
					ChatID:       chatID,
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
		unlock()
	}

	// JOIN every reaper. This is the whole point of the method's contract: when
	// Shutdown returns, every onExit callback the engine owed has RUN — the dying
	// vendor CLIs' deaths are RECORDED (the agent usecase Exits each runner and
	// closes the turn it abandoned), not merely scheduled on a goroutine that the
	// caller is about to pull the databases out from under.
	//
	// No lock is held here (each iteration released its own), and each reaper's first
	// act is to wait on a Done channel the kill loop has already closed, so this
	// converges on the work itself — never on a clock.
	<-reaped
}
