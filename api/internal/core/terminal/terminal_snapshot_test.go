package terminal

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

type wireMsg struct {
	Data     string
	Snapshot bool
}

func decodeMsgs(t *testing.T, conn *recordConn) []wireMsg {
	t.Helper()
	conn.mu.Lock()
	defer conn.mu.Unlock()
	out := make([]wireMsg, 0, len(conn.msgs))
	for _, raw := range conn.msgs {
		data, snapshot, ok := ParseOutputFrame(raw)
		require.True(t, ok, "every output message must be a binary output frame")
		out = append(out, wireMsg{Data: string(data), Snapshot: snapshot})
	}
	return out
}

func TestWritePump_SnapshotIsCoalescingBarrier(t *testing.T) {
	e := newEngine(defaultConfig())
	StopMaintenanceForTest(e)
	conn := newRecordConn()
	ch := make(chan session.OutputFrame, 8)
	done := make(chan struct{})

	ch <- session.OutputFrame{SessionID: "s", Data: []byte("before ")}
	ch <- session.OutputFrame{SessionID: "s", Data: []byte("output")}
	ch <- session.OutputFrame{SessionID: "s", Data: []byte("REDRAW"), Snapshot: true}
	ch <- session.OutputFrame{SessionID: "s", Data: []byte("after")}
	close(ch)

	go e.writePump(conn, ch, notExited, done)
	<-done

	msgs := decodeMsgs(t, conn)
	require.Len(t, msgs, 3)
	assert.Equal(t, wireMsg{Data: "before output", Snapshot: false}, msgs[0],
		"pre-snapshot frames coalesce but must flush before the snapshot")
	assert.Equal(t, wireMsg{Data: "REDRAW", Snapshot: true}, msgs[1],
		"the snapshot must be its own flagged message")
	assert.Equal(t, wireMsg{Data: "after", Snapshot: false}, msgs[2],
		"post-snapshot output must not merge into the snapshot message")
}

func TestWritePump_SnapshotAsFirstFrame(t *testing.T) {
	e := newEngine(defaultConfig())
	StopMaintenanceForTest(e)
	conn := newRecordConn()
	ch := make(chan session.OutputFrame, 2)
	done := make(chan struct{})

	ch <- session.OutputFrame{SessionID: "s", Data: []byte("REDRAW"), Snapshot: true}
	close(ch)

	go e.writePump(conn, ch, notExited, done)
	<-done

	msgs := decodeMsgs(t, conn)
	require.Len(t, msgs, 1)
	assert.Equal(t, wireMsg{Data: "REDRAW", Snapshot: true}, msgs[0])
}

func TestWritePump_SplitRuneIsForwardedVerbatim(t *testing.T) {
	e := newEngine(defaultConfig())
	StopMaintenanceForTest(e)
	conn := newRecordConn()
	ch := make(chan session.OutputFrame, 2)
	done := make(chan struct{})
	go e.writePump(conn, ch, notExited, done)

	ch <- session.OutputFrame{SessionID: "s", Data: []byte{'h', 'i', 0xF0, 0x9F}}
	conn.waitFrames(1)
	ch <- session.OutputFrame{SessionID: "s", Data: []byte{0x9A, 0x80}}
	conn.waitFrames(2)
	close(ch)
	<-done

	msgs := decodeMsgs(t, conn)
	require.Len(t, msgs, 2)
	assert.Equal(t, "hi\xf0\x9f", msgs[0].Data)
	assert.Equal(t, "\x9a\x80", msgs[1].Data)
	assert.Equal(t, "hi🚀", msgs[0].Data+msgs[1].Data)
}

func TestWritePump_SendsExitFrameLast(t *testing.T) {
	e := newEngine(defaultConfig())
	StopMaintenanceForTest(e)
	conn := newRecordConn()
	ch := make(chan session.OutputFrame, 2)
	done := make(chan struct{})
	ch <- session.OutputFrame{SessionID: "s", Data: []byte("bye")}
	close(ch)
	e.writePump(conn, ch, func() (int, bool) { return 7, true }, done)

	conn.mu.Lock()
	defer conn.mu.Unlock()
	require.Len(t, conn.msgs, 2)
	data, _, ok := ParseOutputFrame(conn.msgs[0])
	require.True(t, ok)
	assert.Equal(t, "bye", string(data))
	assert.JSONEq(t, `{"type":"exit","code":7}`, string(conn.msgs[1]))
}

func notExited() (int, bool) { return 0, false }

type recordConn struct {
	mu    sync.Mutex
	msgs  [][]byte
	wrote chan struct{}
}

func newRecordConn() *recordConn { return &recordConn{wrote: make(chan struct{}, 64)} }

func (c *recordConn) waitFrames(n int) {
	for {
		c.mu.Lock()
		got := len(c.msgs)
		c.mu.Unlock()
		if got >= n {
			return
		}
		<-c.wrote
	}
}

func (c *recordConn) WriteMessage(_ int, data []byte) error {
	c.mu.Lock()
	c.msgs = append(c.msgs, append([]byte(nil), data...))
	c.mu.Unlock()
	select {
	case c.wrote <- struct{}{}:
	default:
	}
	return nil
}
func (c *recordConn) ReadMessage() (int, []byte, error) { select {} }
func (c *recordConn) SetWriteDeadline(time.Time) error  { return nil }
func (c *recordConn) Close() error                      { return nil }
