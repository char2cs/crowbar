package session

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
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

// Session is a single live PTY session.
type Session struct {
	id      string
	ptmx    *os.File
	cmd     *exec.Cmd
	ring    *RingBuffer
	mu      sync.Mutex
	clients map[*client]struct{}
	done    chan struct{}
	once    sync.Once
}

// New spawns a PTY subprocess and starts the io pump.
// shell is the binary to exec; cwd is the working directory.
func New(
	id string,
	shell string,
	cwd string,
	env []string,
) (*Session, error) {
	cmd := exec.Command(shell)
	cmd.Dir = cwd
	cmd.Env = env

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("session: pty start: %w", err)
	}

	s := &Session{
		id:      id,
		ptmx:    ptmx,
		cmd:     cmd,
		ring:    newRingBuffer(defaultRingSize),
		clients: make(map[*client]struct{}),
		done:    make(chan struct{}),
	}

	go s.pump()
	return s, nil
}

// ID returns the session identifier.
func (s *Session) ID() string {
	return s.id
}

// Done returns a channel closed when the session has terminated.
func (s *Session) Done() <-chan struct{} {
	return s.done
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

// Write sends data to the PTY stdin.
func (s *Session) Write(
	data []byte,
) error {
	_, err := s.ptmx.Write(data)
	if err != nil {
		return fmt.Errorf("session: write: %w", err)
	}
	return nil
}

// Resize updates the PTY window size.
func (s *Session) Resize(
	cols uint16,
	rows uint16,
) error {
	err := pty.Setsize(s.ptmx, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
	if err != nil {
		return fmt.Errorf("session: resize: %w", err)
	}
	return nil
}

// Kill terminates the PTY process and waits for cleanup.
func (s *Session) Kill() {
	_ = s.ptmx.Close()
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
	s.shutdown()
}

// pump reads PTY stdout, appends to the ring, and fans out to all clients.
func (s *Session) pump() {
	defer s.shutdown()

	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.ring.Write(chunk)
			s.fanOut(chunk)
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
func (s *Session) fanOut(
	chunk []byte,
) {
	frame := OutputFrame{SessionID: s.id, Data: chunk}

	s.mu.Lock()
	defer s.mu.Unlock()

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

// shutdown closes the done channel and all client channels exactly once.
func (s *Session) shutdown() {
	s.once.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		for cl := range s.clients {
			close(cl.send)
		}
		s.clients = make(map[*client]struct{})
		close(s.done)
	})
}
