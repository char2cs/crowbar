package stream_test

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// ledgerSrc is a fake ledger: each Attach replays rows after since, says
// synced, then follows its own live channel until its hold ends. With linger
// set, an ended hold still emits one frame, as a source slow to notice would.
type ledgerSrc struct {
	rows    int64
	missing bool
	linger  bool

	mu       sync.Mutex
	emitted  atomic.Int64
	attached []int64
	lives    chan chan stream.Frame
	ends     chan int64
}

func newLedgerSrc(rows int64) *ledgerSrc {
	return &ledgerSrc{rows: rows, lives: make(chan chan stream.Frame, 128), ends: make(chan int64, 128)}
}

func (l *ledgerSrc) Attach(ctx context.Context, _ string, since int64, emit func(stream.Frame)) error {
	l.mu.Lock()
	l.attached = append(l.attached, since)
	l.mu.Unlock()
	defer func() { l.ends <- since }()
	if l.missing {
		return fmt.Errorf("chat source: %w", problem.New(problem.NotFound, "no chat c9"))
	}
	for seq := since + 1; seq <= l.rows; seq++ {
		emit(stream.Frame{T: stream.FrameRow, Row: &ledger.Row{Seq: seq}})
		l.emitted.Add(1)
	}
	emit(stream.Frame{T: stream.FrameSynced, Seq: l.rows})
	live := make(chan stream.Frame)
	l.lives <- live
	for {
		select {
		case f := <-live:
			emit(f)
		case <-ctx.Done():
			if l.linger {
				emit(<-live)
			}
			return nil
		}
	}
}

// follow is the live channel of the next hold to reach its live frames.
func (l *ledgerSrc) follow(t *testing.T) chan<- stream.Frame {
	t.Helper()
	select {
	case live := <-l.lives:
		return live
	case <-time.After(waitBound):
		require.FailNow(t, "no hold went live")
		return nil
	}
}

// ended waits for one Attach to return.
func (l *ledgerSrc) ended(t *testing.T) {
	t.Helper()
	select {
	case <-l.ends:
	case <-time.After(waitBound):
		require.FailNow(t, "no hold ended")
	}
}

func (l *ledgerSrc) sinces() []int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]int64(nil), l.attached...)
}

func ledgerHub(src *ledgerSrc) *appstream.Hub {
	return newHub(map[stream.ScopeKind]appstream.Source{
		stream.KindChat:     appstream.Following(src),
		stream.KindProjects: newEntities(map[string]string{}),
	}, nil)
}

// A replay past even the hard bound reaches a writer that drains slowly: the
// replay waits for room instead of closing the connection (index R8).
func TestHub_ALedgerReplayLargerThanTheQueueIsDelivered(t *testing.T) {
	rows := stream.HardQueueSize + stream.QueueSize
	src := newLedgerSrc(int64(rows))
	hub := ledgerHub(src)
	conn := newFakeConn()
	conn.throttle = true
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "chat:c1")
	// The writer holds still until the replay has filled the queue it waits on.
	require.Eventually(t, func() bool { return src.emitted.Load() >= stream.QueueSize },
		waitBound, time.Millisecond)
	go func() {
		for range rows + 1 {
			select {
			case conn.allow <- struct{}{}:
			case <-conn.closed:
				return
			}
		}
	}()
	seen := conn.until(t, syncedOn("chat:c1"))

	require.Len(t, seen, rows+1)
	assert.Equal(t, int64(rows), seen[rows].Seq)
	assert.Equal(t, 0, conn.closeCode())
}

// Live ledger frames count toward the bound: a client that stops reading is told to resync.
func TestHub_ALiveLedgerOverflowClosesWithResync(t *testing.T) {
	src := newLedgerSrc(0)
	hub := ledgerHub(src)
	conn := newFakeConn()
	conn.throttle = true
	done := served(t, hub, conn)
	conn.allow <- struct{}{} // the synced frame, then the writer holds still
	conn.sub(t, "chat:c1")
	live := src.follow(t)

	go func() {
		for i := range stream.QueueSize + 2 {
			select {
			case live <- stream.Frame{T: stream.FrameRow, Row: &ledger.Row{Seq: int64(i + 1)}}:
			case <-conn.closed:
				return
			}
		}
	}()

	ended(t, done)
	assert.Equal(t, stream.CloseResync, conn.closeCode())
}

func TestHub_ALedgerScopeFollowsLiveFramesBesideOtherScopes(t *testing.T) {
	src := newLedgerSrc(2)
	hub := ledgerHub(src)
	conn := newFakeConn()
	served(t, hub, conn)

	replay := conn.sub(t, "chat:c1")
	require.Len(t, replay, 2)
	assert.Equal(t, "chat:c1", replay[0].Scope)
	conn.sub(t, "projects") // another scope is served while the ledger follows
	src.follow(t) <- stream.Frame{T: stream.FrameRow, Row: &ledger.Row{Seq: 3}}

	live := conn.next(t)
	assert.Equal(t, stream.Frame{T: stream.FrameRow, Scope: "chat:c1", Row: &ledger.Row{Seq: 3}}, live)
}

func TestHub_ALedgerCloseEndsItsHoldOnly(t *testing.T) {
	src := newLedgerSrc(0)
	hub := ledgerHub(src)
	conn := newFakeConn()
	served(t, hub, conn)
	conn.sub(t, "chat:c1")
	conn.sub(t, "projects")

	src.follow(t) <- stream.Closed("", stream.ReasonDeleted, nil)
	assert.Equal(t, stream.Closed("chat:c1", stream.ReasonDeleted, nil), conn.next(t))
	src.ended(t)
	hub.Entity(stream.Projects, stream.KindOperation, "op1", "x")
	assert.Equal(t, "op1", conn.next(t).ID, "the projects scope still hears its frames")

	conn.sub(t, "chat:c1") // a closed hold is a new hold on the next sub
	assert.Equal(t, []int64{0, 0}, src.sinces())
}

func TestHub_AnUnknownLedgerIsRefused(t *testing.T) {
	src := newLedgerSrc(0)
	src.missing = true
	hub := ledgerHub(src)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "chat:c9")
	closed := conn.next(t)
	assert.Equal(t, stream.FrameClosed, closed.T)
	assert.Equal(t, stream.ReasonRefused, closed.Reason)
	assert.Equal(t, problem.NotFound, closed.Problem.Code)
	conn.sub(t, "projects")
}

func TestHub_ASubOfAFollowedLedgerAttachesOnce(t *testing.T) {
	src := newLedgerSrc(1)
	hub := ledgerHub(src)
	conn := newFakeConn()
	served(t, hub, conn)
	conn.sub(t, "chat:c1")

	conn.send(t, stream.OpSub, "chat:c1")
	conn.sub(t, "projects")
	assert.Equal(t, []int64{0}, src.sinces())
}

// A live frame of a hold let go of never reaches the client, even after it
// subs the same ledger again.
func TestHub_UnsubEndsTheFollowAndItsFrames(t *testing.T) {
	src := newLedgerSrc(0)
	src.linger = true
	hub := ledgerHub(src)
	conn := newFakeConn()
	served(t, hub, conn)
	conn.sub(t, "chat:c1")
	first := src.follow(t)
	conn.send(t, stream.OpUnsub, "chat:c1")
	conn.sub(t, "chat:c1")
	second := src.follow(t)

	first <- stream.Frame{T: stream.FrameRow, Row: &ledger.Row{Seq: 99}}
	src.ended(t)
	second <- stream.Frame{T: stream.FrameRow, Row: &ledger.Row{Seq: 1}}
	seen := conn.until(t, func(f stream.Frame) bool { return f.Row != nil && f.Row.Seq == 1 })

	assert.Len(t, seen, 1, "the hold let go of told the client nothing")
}

func TestHub_LedgerHoldsLeaveNoGoroutineBehind(t *testing.T) {
	src := newLedgerSrc(1)
	hub := ledgerHub(src)
	conn := newFakeConn()
	served(t, hub, conn)
	conn.sub(t, "projects")
	before := runtime.NumGoroutine()

	for i := range 100 {
		conn.sub(t, fmt.Sprintf("chat:c%d", i%5))
		conn.send(t, stream.OpUnsub, fmt.Sprintf("chat:c%d", i%5))
	}
	conn.sub(t, "projects")

	require.Eventually(t, func() bool { return runtime.NumGoroutine() <= before+1 },
		waitBound, time.Millisecond, "goroutines grew from %d", before)
}
