package stream

import (
	"context"
	"log/slog"
	"sync"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// Conn is one client's socket as the hub drives it.
type Conn interface {
	ReadJSON(v any) error
	WriteJSON(v any) error
	CloseWith(code int, reason string) error
}

const (
	closeGoingAway       = 1001
	closeUnsupportedData = 1003
)

type outgoing struct {
	frame stream.Frame
	live  bool
}

// connection orders every frame for one client. Live frames are bounded by
// QueueSize; every frame, snapshots too, by HardQueueSize (index R8).
type connection struct {
	conn Conn
	// scopes is what this connection holds, guarded by the hub's lock.
	scopes map[string]stream.Scope
	// resources is each scope whose resources it holds, its Serve goroutine's alone.
	resources map[string]stream.Scope
	// follows is each ledger scope it follows, guarded by the hub's lock.
	follows map[string]*follow

	mu      sync.Mutex
	pending []outgoing
	live    int
	ended   bool
	// snapshotting is each scope whose synced frame is still queued.
	snapshotting map[string]bool

	wake chan struct{}
	// room hears each frame the writer takes, for a replay waiting to queue.
	room chan struct{}
	done chan struct{}
	once sync.Once
}

func newConnection(conn Conn) *connection {
	return &connection{
		conn:         conn,
		scopes:       map[string]stream.Scope{},
		resources:    map[string]stream.Scope{},
		follows:      map[string]*follow{},
		snapshotting: map[string]bool{},
		wake:         make(chan struct{}, 1),
		room:         make(chan struct{}, 1),
		done:         make(chan struct{}),
	}
}

// push queues f; false means the queue is full and the connection must resync.
func (c *connection) push(f stream.Frame, live bool) bool {
	c.mu.Lock()
	if c.ended {
		c.mu.Unlock()
		return true
	}
	if live && c.live >= stream.QueueSize || len(c.pending) >= stream.HardQueueSize {
		c.mu.Unlock()
		return false
	}
	c.pending = append(c.pending, outgoing{frame: f, live: live})
	if live {
		c.live++
	}
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
	return true
}

func (c *connection) pop() (stream.Frame, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pending) == 0 {
		return stream.Frame{}, false
	}
	next := c.pending[0]
	c.pending[0] = outgoing{}
	c.pending = c.pending[1:]
	if next.live {
		c.live--
	}
	if next.frame.T == stream.FrameSynced {
		delete(c.snapshotting, next.frame.Scope)
	}
	select {
	case c.room <- struct{}{}:
	default:
	}
	return next.frame, true
}

// await blocks while QueueSize frames wait; false once the connection or ctx ends.
func (c *connection) await(ctx context.Context) bool {
	for {
		c.mu.Lock()
		ended, full := c.ended, len(c.pending) >= stream.QueueSize
		c.mu.Unlock()
		if ended {
			return false
		}
		if !full {
			return true
		}
		select {
		case <-c.room:
		case <-c.done:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

// beginSnapshot marks scope's snapshot queued; false if a held scope's already is.
func (c *connection) beginSnapshot(scope string, held bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if held && c.snapshotting[scope] {
		return false
	}
	c.snapshotting[scope] = true
	return true
}

// endSnapshot forgets a snapshot that ended without its synced frame.
func (c *connection) endSnapshot(scope string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.snapshotting, scope)
}

// write drains the queue to the socket until the connection ends or a write fails.
func (c *connection) write() {
	for {
		select {
		case <-c.done:
			return
		case <-c.wake:
		}
		for f, ok := c.pop(); ok; f, ok = c.pop() {
			if err := c.conn.WriteJSON(f); err != nil {
				c.end(closeGoingAway, "write failed")
				return
			}
		}
	}
}

// end closes the socket once, which also ends Serve's read.
func (c *connection) end(code int, reason string) {
	c.once.Do(func() {
		c.mu.Lock()
		c.ended = true
		c.pending = nil
		c.mu.Unlock()
		close(c.done)
		// The writer may be stuck in a write: close from aside, never under the hub's lock.
		go func() {
			defer safego.Recover("stream.close")
			if err := c.conn.CloseWith(code, reason); err != nil {
				slog.Debug("stream: close a connection", "code", code, "err", err)
			}
		}()
	})
}
