package session

import (
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

// OutputFrame is a chunk of PTY output for attached clients. Snapshot marks a self-contained redraw: the
// client must RESET its buffer before applying Data (attach redraw, post-resize keyframe).
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
	newModel   func(cols, rows, scrollback int) (model.TerminalModel, model.Serializer)
	mu         sync.Mutex
	clients    map[*client]struct{}
	done       chan struct{}
	once       sync.Once
	dirty      bool
	// screenGen advances whenever the model takes something that can change the screen, so an observer can
	// skip re-rendering an unchanged one (Session.ScreenText). Not s.dirty: that tracks the persisted blob
	// and is consumed by Snapshot, while this only goes up and serves any number of readers.
	screenGen uint64
	exitCode  int
	cwd       string
	shell     string
	profileID string
	// command marks a NewCommand session (an explicit-argv CLI, not a login shell); set once before any
	// goroutine starts, so lock-free reads are safe. It can never survive Suspend's restore, so it is
	// excluded from suspend/evict (SuspendEligible).
	command bool
	// lastBlob caches the last live-session serialized blob (header + redraw) so a
	// cadence flush of an unchanged session reuses it and skips the grid render (§8.4).
	// It is reclaimable under memory pressure (DropCachedBlob, §9.4).
	lastBlob []byte
	// modelPanics counts recovered session-level model panics (each rebuilds the model), surfaced via Stats;
	// vtModel.Write's own parse panics are not counted.
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
	// sampleForegroundLocked is the foreground probe pumpStep runs last; a field so a test can pin the
	// fan-out, model-write, foreground-sample ordering.
	sampleForegroundLocked func()
	// now is the frame clock's time source, a field read under s.mu so a test can pin it without a racy
	// package var: proving a burst lands inside ONE interval cannot rely on running fast under -race.
	// Production uses time.Now.
	now func() time.Time
	// pumpNotify is a test-only seam (PumpNotifyForTest) that pumpStep signals once a chunk is processed, so
	// a test blocks on real progress instead of sleeping. The send is non-blocking; see notifyPumpLocked.
	pumpNotify chan struct{}
	// Clients receive only model-derived diff/keyframe frames, never raw PTY bytes; a panicking model is
	// rebuilt and the next frame forced to a keyframe. Guarded by s.mu.
	emitter *model.DiffEmitter
	// Adaptive frame clock state (see scheduleEmitLocked). Guarded by s.mu; the timer callback re-locks.
	lastEmitAt time.Time
	emitTimer  *time.Timer

	// Theme-notify dedupe: a CSI ?997;n report goes out on the first subscribed push and each light<->dark
	// flip, never on a same-polarity tweak. Guarded by s.mu.
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
