package terminal

// writePump snapshot-barrier coverage: a Snapshot frame is a self-contained
// redraw the client applies onto a RESET buffer, so it must never be merged
// into (or split across) incremental output messages.

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

type wireMsg struct {
	Data     string `json:"data"`
	Snapshot bool   `json:"snapshot"`
}

func decodeMsgs(t *testing.T, conn *recordConn) []wireMsg {
	t.Helper()
	conn.mu.Lock()
	defer conn.mu.Unlock()
	out := make([]wireMsg, 0, len(conn.msgs))
	for _, raw := range conn.msgs {
		var m wireMsg
		require.NoError(t, json.Unmarshal(raw, &m))
		out = append(out, m)
	}
	return out
}

// TestWritePump_SnapshotIsCoalescingBarrier queues raw output, a snapshot, and
// more raw output before the pump runs: the snapshot must come through as its
// own flagged message, with the surrounding raw bytes in separate unflagged
// messages, in order.
func TestWritePump_SnapshotIsCoalescingBarrier(t *testing.T) {
	e, _ := newCoverEngine(t)
	conn := newRecordConn()
	ch := make(chan session.OutputFrame, 8)
	done := make(chan struct{})

	ch <- session.OutputFrame{SessionID: "s", Data: []byte("before ")}
	ch <- session.OutputFrame{SessionID: "s", Data: []byte("output")}
	ch <- session.OutputFrame{SessionID: "s", Data: []byte("REDRAW"), Snapshot: true}
	ch <- session.OutputFrame{SessionID: "s", Data: []byte("after")}
	close(ch)

	go e.writePump(conn, "s", ch, done)
	// Block on the real signal. A hand-rolled deadline here would only be a second,
	// weaker definition of "too slow"; if this never fires it is a hang, and `go test
	// -timeout` reports it with the blocked stack.
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

// TestWritePump_SnapshotAsFirstFrame covers the leading-snapshot branch (the
// attach redraw): one flagged message, nothing merged.
func TestWritePump_SnapshotAsFirstFrame(t *testing.T) {
	e, _ := newCoverEngine(t)
	conn := newRecordConn()
	ch := make(chan session.OutputFrame, 2)
	done := make(chan struct{})

	ch <- session.OutputFrame{SessionID: "s", Data: []byte("REDRAW"), Snapshot: true}
	close(ch)

	go e.writePump(conn, "s", ch, done)
	// Block on the real signal. A hand-rolled deadline here would only be a second,
	// weaker definition of "too slow"; if this never fires it is a hang, and `go test
	// -timeout` reports it with the blocked stack.
	<-done

	msgs := decodeMsgs(t, conn)
	require.Len(t, msgs, 1)
	assert.Equal(t, wireMsg{Data: "REDRAW", Snapshot: true}, msgs[0])
}

// TestWritePump_SnapshotDropsHeldBackPartialRune: a held-back incomplete UTF-8
// tail belongs to the pre-snapshot stream the client reset supersedes — it
// must be dropped, never prepended to post-snapshot output.
func TestWritePump_SnapshotDropsHeldBackPartialRune(t *testing.T) {
	e, _ := newCoverEngine(t)
	conn := newRecordConn()
	ch := make(chan session.OutputFrame, 2)
	done := make(chan struct{})
	go e.writePump(conn, "s", ch, done)

	// A lone frame ending in a dangling 4-byte-rune lead byte: flushed as "hi",
	// 0xF0 held back as pending.
	// Each step blocks on writePump's OWN write signal (conn.WriteMessage), so the frames are
	// ordered by observation rather than by out-polling a 5 ms timer. This ordering is the
	// whole point of the test: the held-back partial rune must not survive the snapshot.
	ch <- session.OutputFrame{SessionID: "s", Data: []byte{'h', 'i', 0xF0}}
	conn.waitFrames(1)

	// The snapshot arrives next; then post-snapshot output.
	ch <- session.OutputFrame{SessionID: "s", Data: []byte("REDRAW"), Snapshot: true}
	conn.waitFrames(2)
	ch <- session.OutputFrame{SessionID: "s", Data: []byte("after")}
	conn.waitFrames(3)
	close(ch)
	<-done

	msgs := decodeMsgs(t, conn)
	require.Len(t, msgs, 3)
	assert.Equal(t, "hi", msgs[0].Data)
	assert.Equal(t, wireMsg{Data: "REDRAW", Snapshot: true}, msgs[1])
	assert.Equal(t, "after", msgs[2].Data,
		"the pending 0xF0 must be dropped at the snapshot barrier, not prepended")
}
