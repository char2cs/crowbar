package session

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

const clientSendBuf = 256

// OutputFrame is a chunk of PTY output delivered to attached clients.
type OutputFrame struct {
	SessionID string
	Data      []byte
}

// client represents one attached WebSocket subscriber.
type client struct {
	send chan OutputFrame
}

// Session is a single PTY session. It may be live (ptmx != nil) or suspended
// (ptmx == nil, created via NewPlaceholder).
type Session struct {
	id         string
	ptmx       *os.File
	cmd        *exec.Cmd
	ring       *RingBuffer
	mu         sync.Mutex
	flushMu    sync.Mutex // used by engine flush in a later step
	clients    map[*client]struct{}
	done       chan struct{}
	once       sync.Once
	suspending bool
	exitCode   int // -1 until shutdown; 0 on clean exit; >0 or -1 on error/kill
	cwd        string
	shell      string
	profileID  string
}

// newSession allocates a Session with the given PTY resources. The pump
// goroutine is NOT started here; callers are responsible for calling go s.pump()
// (or not, for placeholder sessions).
func newSession(id, shell, cwd, profileID string, ptmx *os.File, cmd *exec.Cmd) *Session {
	return &Session{
		id:        id,
		ptmx:      ptmx,
		cmd:       cmd,
		ring:      newRingBuffer(defaultRingSize),
		clients:   make(map[*client]struct{}),
		done:      make(chan struct{}),
		cwd:       cwd,
		shell:     shell,
		profileID: profileID,
		exitCode:  -1, // unknown until shutdown captures it
	}
}

// New spawns a PTY subprocess and starts the io pump.
// shell is the binary to exec; cwd is the working directory; profileID is the
// terminal profile identifier (empty string when not yet known).
func New(
	id string,
	shell string,
	cwd string,
	profileID string,
	env []string,
) (*Session, error) {
	cmd := exec.Command(shell)
	cmd.Dir = cwd
	cmd.Env = env

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("session: pty start: %w", err)
	}

	s := newSession(id, shell, cwd, profileID, ptmx, cmd)
	go s.pump()
	return s, nil
}

// NewRestored spawns a PTY subprocess like New, but pre-loads scrollback into the
// ring buffer BEFORE starting the pump goroutine. This ensures that the restored
// scrollback appears before the new shell's first output in the ring's chronological
// order, so Attach() replays old data ahead of new data.
func NewRestored(
	id string,
	shell string,
	cwd string,
	profileID string,
	env []string,
	scrollback []byte,
) (*Session, error) {
	cmd := exec.Command(shell)
	cmd.Dir = cwd
	cmd.Env = env

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("session: pty start (restore): %w", err)
	}

	s := newSession(id, shell, cwd, profileID, ptmx, cmd)
	if len(scrollback) > 0 {
		// Write BEFORE pump starts so the ring snapshot stays chronological.
		s.ring.Write(scrollback)
	}
	go s.pump()
	return s, nil
}

// NewPlaceholder builds a suspended Session with no live PTY. The scrollback
// bytes are loaded into the ring buffer so Attach() replays them. The done
// channel is open — it is closed only when Kill() is called. No pump goroutine
// is started; Attach() on a placeholder does not panic.
func NewPlaceholder(
	id string,
	shell string,
	cwd string,
	profileID string,
	scrollback []byte,
) *Session {
	r := newRingBuffer(defaultRingSize)
	if len(scrollback) > 0 {
		r.Write(scrollback)
	}
	return &Session{
		id:        id,
		ring:      r,
		clients:   make(map[*client]struct{}),
		done:      make(chan struct{}),
		cwd:       cwd,
		shell:     shell,
		profileID: profileID,
		exitCode:  -1, // unknown; placeholder has no process
		// ptmx and cmd intentionally nil — State() returns "suspended"
	}
}

// ID returns the session identifier.
func (s *Session) ID() string {
	return s.id
}

// Done returns a channel closed when the session has terminated.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// IsLive reports whether the PTY is still open (ptmx != nil).
func (s *Session) IsLive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ptmx != nil
}

// AttachedCount returns the number of currently attached clients.
func (s *Session) AttachedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}

// State returns one of "active", "detached", or "suspended".
//
//   - "suspended"  — ptmx is nil (placeholder or after Kill)
//   - "active"     — ptmx is live and at least one client is attached
//   - "detached"   — ptmx is live but no clients are attached
func (s *Session) State() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptmx == nil {
		return "suspended"
	}
	if len(s.clients) > 0 {
		return "active"
	}
	return "detached"
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

// FlushMu returns the dedicated flush mutex for use by the engine in a later
// step. It is separate from the main session mutex (s.mu) so that bulk
// scrollback flushes do not block fan-out.
func (s *Session) FlushMu() *sync.Mutex {
	return &s.flushMu
}

// IsIdle reports whether the shell is idle (no foreground child process).
// This is a thin, platform-agnostic wrapper around isIdleLocked.
func (s *Session) IsIdle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isIdleLocked()
}

// Attach registers a new client and returns its channel, pre-filled with the
// ring-buffer snapshot. The snapshot and registration happen under a single
// mutex acquisition so no output is lost or duplicated.
func (s *Session) Attach() (<-chan OutputFrame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.done:
		return nil, fmt.Errorf("session: attach: session %s is dead", s.id)
	default:
	}

	cl := &client{send: make(chan OutputFrame, clientSendBuf)}

	snap := s.ring.Snapshot()
	if len(snap) > 0 {
		cl.send <- OutputFrame{SessionID: s.id, Data: snap}
	}

	s.clients[cl] = struct{}{}
	return cl.send, nil
}

// Detach removes a client from the fan-out set and closes its channel.
func (s *Session) Detach(
	ch <-chan OutputFrame,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detachLocked(ch)
}

// detachLocked removes the client whose send channel matches ch.
// Caller must hold s.mu.
func (s *Session) detachLocked(
	ch <-chan OutputFrame,
) {
	for cl := range s.clients {
		if ch == (<-chan OutputFrame)(cl.send) {
			delete(s.clients, cl)
			close(cl.send)
			return
		}
	}
}

// Write sends data to the PTY stdin. Returns an error if the PTY is not live.
func (s *Session) Write(
	data []byte,
) error {
	s.mu.Lock()
	ptmx := s.ptmx
	s.mu.Unlock()
	if ptmx == nil {
		return fmt.Errorf("session: write: session not live")
	}
	_, err := ptmx.Write(data)
	if err != nil {
		return fmt.Errorf("session: write: %w", err)
	}
	return nil
}

// Resize updates the PTY window size. Returns an error if the PTY is not live.
func (s *Session) Resize(
	cols uint16,
	rows uint16,
) error {
	s.mu.Lock()
	ptmx := s.ptmx
	s.mu.Unlock()
	if ptmx == nil {
		return fmt.Errorf("session: resize: session not live")
	}
	err := pty.Setsize(ptmx, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
	if err != nil {
		return fmt.Errorf("session: resize: %w", err)
	}
	return nil
}

// Kill terminates the PTY process. For placeholder sessions (ptmx == nil) it
// only calls shutdown(). For live sessions it acquires s.mu, closes the PTY,
// kills the child, sets ptmx = nil, releases s.mu, then calls shutdown().
// s.mu must NOT be held across shutdown() because shutdown() re-acquires it
// inside once.Do — that would self-deadlock.
func (s *Session) Kill() {
	s.mu.Lock()
	if s.ptmx == nil {
		// Placeholder: no PTY to close.
		s.mu.Unlock()
		s.shutdown()
		return
	}
	_ = s.ptmx.Close()
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	s.ptmx = nil
	s.mu.Unlock()

	// shutdown() re-acquires s.mu inside once.Do — must not hold it here.
	s.shutdown()
}

// pumpStep is the production critical section for one PTY output chunk: it holds
// s.mu across both ring.Write and fanOutLocked so that Attach() can never observe
// a chunk in the ring snapshot but not yet in the fan-out (which would cause the
// client to receive the chunk twice). Lock order: s.mu → ring.mu (same as Attach).
// Called from pump() and, via PumpChunkForTest, from the regression test — so a
// regression that removes the lock will break the race-detector test.
// Also scans the chunk for OSC 7 sequences to update s.cwd (best-effort).
func (s *Session) pumpStep(chunk []byte) {
	s.mu.Lock()
	if path, ok := parseLastOSC7(chunk); ok {
		s.cwd = path
	}
	s.ring.Write(chunk)   // ring.mu nested under s.mu — same order Attach uses
	s.fanOutLocked(chunk) // assumes s.mu held
	s.mu.Unlock()
}

// pump reads PTY stdout and delivers each chunk via pumpStep.
func (s *Session) pump() {
	// A panic here must not crash the daemon; cleanup (shutdown) still runs on the
	// unwind because its defer is registered after this recover boundary.
	defer safego.Recover("terminal.session.pump")
	defer s.shutdown()

	// Capture ptmx into a local variable once, under s.mu, so that Kill()
	// can safely write s.ptmx = nil later without racing the field read in
	// the hot loop below. The underlying *os.File is unaffected — Close() on
	// it will cause Read() to return an error, which terminates the loop.
	s.mu.Lock()
	ptmx := s.ptmx
	s.mu.Unlock()
	if ptmx == nil {
		return // placeholder: no PTY to read from
	}

	buf := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.pumpStep(chunk)
		}
		if err != nil {
			if !isNormalPTYClose(err) {
				_, _ = fmt.Fprintf(os.Stderr, "terminal: session %s: pump error: %v\n", s.id, err)
			}
			return
		}
	}
}

// isNormalPTYClose reports whether err is the expected error when the shell exits.
// On Linux the PTY master returns EIO; on macOS it returns io.EOF.
func isNormalPTYClose(
	err error,
) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.EIO
	}
	return false
}

// fanOut delivers a chunk to all currently attached clients.
// Clients whose channel is full are disconnected (drop-on-overflow).
// This is a thin convenience wrapper for callers that do not already hold s.mu.
func (s *Session) fanOut(
	chunk []byte,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fanOutLocked(chunk)
}

// fanOutLocked delivers a chunk to all currently attached clients.
// Clients whose channel is full are disconnected (drop-on-overflow).
// Caller must hold s.mu.
func (s *Session) fanOutLocked(
	chunk []byte,
) {
	frame := OutputFrame{SessionID: s.id, Data: chunk}

	var overflow []*client
	for cl := range s.clients {
		select {
		case cl.send <- frame:
		default:
			overflow = append(overflow, cl)
		}
	}

	for _, cl := range overflow {
		delete(s.clients, cl)
		close(cl.send)
	}
}

// shutdown reaps the child process and closes the done + client channels exactly
// once. The cmd.Wait() here is the ONLY reap: without it a shell that exits on
// its own (the common case — the user types `exit`) is never waited on and leaks
// a zombie holding a PID slot until the daemon dies. once guards against a double
// Wait when both Kill() and pump()'s exit reach shutdown. Wait runs before the
// lock so a slow child exit never blocks fan-out cleanup.
func (s *Session) shutdown() {
	s.once.Do(func() {
		code := -1
		if s.cmd != nil {
			err := s.cmd.Wait()
			if err == nil {
				code = 0
			} else {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					code = exitErr.ExitCode()
				}
				// else: killed with signal → -1 (unknown)
			}
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		s.exitCode = code
		for cl := range s.clients {
			close(cl.send)
		}
		s.clients = make(map[*client]struct{})
		close(s.done)
	})
}

// Snapshot returns a copy of all bytes currently in the ring buffer, in
// chronological order. It is safe for concurrent use and does not acquire s.mu
// (only ring.mu is taken internally by RingBuffer.Snapshot).
func (s *Session) Snapshot() []byte {
	return s.ring.Snapshot()
}

// ExitCode returns the process exit code captured by shutdown(). It returns -1
// if the process has not yet exited or was killed with a signal (unknown code).
func (s *Session) ExitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode
}

// BeginSuspendIfEligible atomically checks whether this session is eligible for
// suspend and, if so, sets the suspending flag under s.mu. A session is eligible
// only if it has no attached clients, is not already suspending, and is idle.
// Returns true if the caller should proceed with the suspend; false otherwise.
// This is an atomic check-and-set that closes the TOCTOU window between the
// idle-check and the actual kill.
func (s *Session) BeginSuspendIfEligible() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.clients) > 0 || s.suspending || !s.isIdleLocked() {
		return false
	}
	s.suspending = true
	return true
}

// Suspending reports whether the suspend flag has been set by BeginSuspendIfEligible.
// reapOnDone checks this flag to distinguish an intentional suspend-kill from a
// real process exit, and skips reg.Remove + fireEnded for suspend.
func (s *Session) Suspending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.suspending
}

// parseLastOSC7 scans b for OSC 7 sequences of the form
//
//	ESC ] 7 ; file://[host]/path BEL
//
// and returns the decoded path from the last match found. Best-effort: partial
// sequences that span chunk boundaries are silently ignored.
func parseLastOSC7(b []byte) (string, bool) {
	prefix := []byte("\x1b]7;")
	last := ""
	found := false

	for {
		idx := bytes.Index(b, prefix)
		if idx < 0 {
			break
		}
		b = b[idx+len(prefix):]

		// Find the terminator: BEL (0x07) or ST (ESC \).
		end := -1
		for i := 0; i < len(b); i++ {
			if b[i] == '\x07' {
				end = i
				break
			}
			if b[i] == '\x1b' && i+1 < len(b) && b[i+1] == '\\' {
				end = i
				break
			}
		}
		if end < 0 {
			break // no terminator in this chunk — skip (best-effort)
		}

		uri := string(b[:end])
		b = b[end+1:]

		if !strings.HasPrefix(uri, "file://") {
			continue
		}

		parsed, err := url.Parse(uri)
		if err != nil || parsed.Path == "" {
			continue
		}

		last = parsed.Path
		found = true
	}

	return last, found
}
