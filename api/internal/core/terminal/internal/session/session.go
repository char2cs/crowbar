package session

import (
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

// OutputFrame is a chunk of PTY output delivered to attached clients.
//
// Snapshot marks a self-contained ground-state redraw (the serialized model)
// rather than incremental PTY bytes: the client must RESET its local buffer
// before applying Data, replacing whatever it accumulated — the mechanism
// behind both the attach redraw and the post-resize keyframe.
type OutputFrame struct {
	SessionID string
	Data      []byte
	Snapshot  bool
}

// client represents one attached WebSocket subscriber.
type client struct {
	send chan OutputFrame
}

// Session is a single PTY process and its screen model. It is born live and dies exactly
// once (Done closes); it has no notion of being suspended or restored — that lifecycle
// belongs to the engine, which replaces a dead Session with a new one on restore.
type Session struct {
	id         string
	ptmx       *os.File
	cmd        *exec.Cmd
	model      model.TerminalModel
	serializer model.Serializer
	mu         sync.Mutex
	clients    map[*client]struct{}
	done       chan struct{}
	once       sync.Once
	dirty      bool
	// screenGen advances every time the model TAKES something that can change what
	// is on screen — a PTY chunk, a daemon-authored injection, a resize, a
	// foreground-app teardown.
	//
	// Deliberately NOT s.dirty, which it superficially resembles. dirty is about the
	// PERSISTED BLOB: DropCachedBlob sets it with nothing on screen having changed,
	// and Snapshot consumes it, so an observer reading dirty would both see phantom
	// changes and race the flusher for the one bit. This counter is consumed by
	// nobody and only ever goes up, so any number of readers can each remember their
	// own last value.
	//
	// It exists so a screen observer can skip the work of rendering and scanning a
	// screen that has not moved since it last looked — see Session.ScreenText.
	screenGen uint64
	exitCode  int
	cwd       string
	shell     string
	profileID string
	// command marks a session spawned via NewCommand — an explicit-argv agentic vendor
	// CLI (claude/codex), as opposed to a login shell. It is set exactly once in
	// NewCommand before any goroutine starts and never mutated again, so reading it
	// without s.mu is race-safe. A command session can never survive Suspend's PTY
	// teardown+restore (restore would exec.Command the joined argv string as a bogus
	// binary), so it must be excluded from the maintenance sweep's suspend/evict
	// eligibility entirely (SuspendEligible below).
	command bool
	// lastBlob caches the last live-session serialized blob (header + redraw) so a
	// cadence flush of an unchanged session reuses it and skips the grid render (§8.4).
	// It is reclaimable under memory pressure (DropCachedBlob, §9.4).
	lastBlob []byte
	// modelPanics counts recovered SESSION-LEVEL model-access panics — the §8.5
	// backstops around Resize/Serialize/Emit/Prime/teardown, each of which rebuilds
	// the model (resetModelLocked) — surfaced via Stats and never fatal. It does NOT
	// count vtModel.Write's internal parse panics, which the model recovers itself.
	modelPanics int
	// cols/rows/scrollback are the PTY's current dimensions — what a rebuilt model is
	// born at. Guarded by s.mu.
	cols, rows, scrollback int
	// replySink is the model's device-query response sink (startResponseSink), kept so
	// a rebuilt model answers queries through the same queue.
	replySink func([]byte)
	// lastForegroundPgid latches the previous foreground process-group sample so the
	// app→shell return edge fires OnForegroundReset exactly once (§11.1).
	lastForegroundPgid int
	lastFgSampleAt     time.Time
	// sampleForegroundLocked is the §11.1 site #1 foreground-process-group probe that
	// pumpStep runs strictly LAST in its critical section (after fan-out and the model
	// write). It is a field — wired to checkForegroundResetLocked in newBareSession —
	// solely so a test can substitute a deterministic hooked sampler to pin the
	// fan-out → model-write → foreground-sample ordering the spec mandates (§11.1 site #1).
	sampleForegroundLocked func()
	// now is the frame clock's time source (see scheduleEmitLocked). It is a FIELD, read only
	// under s.mu, so that a test can pin it without introducing the cross-goroutine write a
	// package-level var would (the trailing emit timer reads the clock from its own goroutine).
	//
	// It exists because the coalescing decision — "emit this chunk now, or fold it into the
	// pending trailing frame?" — is a comparison against the wall clock. A test asserting
	// "a burst landing inside ONE interval yields exactly ONE frame" must be able to GUARANTEE
	// the burst really was inside one interval, and it cannot do that by running fast: under
	// -race, 50 pumpStep cycles were measured at 9-19ms against an 8ms window. The burst would
	// cross the boundary, take the immediate-emit branch, and produce an extra frame — a
	// failure caused by the test's own runtime rather than by the logic under test. Pinning the
	// clock removes the guess. Production leaves it as time.Now.
	now func() time.Time
	// pumpNotify is a test-only observability seam (read via PumpNotifyForTest): pumpStep
	// signals it, last in its critical section, once a chunk is fully processed. It exists
	// so a test can BLOCK on real pump progress instead of sleeping and hoping — the PTY is
	// an asynchronous source, and a duration is a guess about fork/exec speed, not a wait.
	// Production never receives from it; the send is non-blocking, so an absent listener
	// costs the pump nothing and changes no behaviour. See notifyPumpLocked.
	pumpNotify chan struct{}
	// Model-driven output (spec 2026-07-03): clients receive model-derived
	// diff/keyframe frames, never raw PTY bytes — there is no raw fallback. A model
	// method that escapes a panic gets the model rebuilt and the next frame forced to
	// a keyframe (resetModelLocked). emitter state is guarded by s.mu like the model.
	emitter *model.DiffEmitter
	// Adaptive frame clock (spec §3.3): emits immediately when the last emit
	// is older than minEmitInterval (interactive echo stays un-batched), else
	// arms one trailing timer at the boundary so bursts coalesce. Guarded by
	// s.mu; the timer callback re-locks.
	lastEmitAt time.Time
	emitTimer  *time.Timer

	// Theme-notify dedupe (SetTheme): themeEmitted latches once a CSI ?997;n report has
	// been emitted, themeEmittedDark records the last-emitted polarity. Together they
	// gate the report to the first subscribed push and every subsequent light<->dark
	// FLIP, so an unrelated same-polarity theme tweak never spams a running app with
	// redundant re-query triggers. Guarded by s.mu.
	themeEmitted     bool
	themeEmittedDark bool
}

// ID returns the session identifier.
func (s *Session) ID() string {
	return s.id
}

// Done returns a channel closed when the session has terminated.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// AttachedCount returns the number of currently attached clients.
func (s *Session) AttachedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}

// CWD returns the last known working directory (updated from OSC 7 sequences).
func (s *Session) CWD() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cwd
}

// Shell returns the shell binary path.
func (s *Session) Shell() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shell
}

// ProfileID returns the terminal profile identifier.
func (s *Session) ProfileID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profileID
}

// IsCommand reports whether this session was spawned via NewCommand (an explicit-argv
// agentic vendor CLI) rather than a login shell. command is set once, before spawn's
// goroutine starts, and never mutated again, so it is safe to read without s.mu.
func (s *Session) IsCommand() bool {
	return s.command
}

// IsIdle reports whether the shell is idle (no foreground child process).
func (s *Session) IsIdle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isIdleLocked()
}

// ExitCode returns the process exit code captured by shutdown(). -1 if not yet exited or
// killed with a signal (unknown code).
func (s *Session) ExitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode
}
