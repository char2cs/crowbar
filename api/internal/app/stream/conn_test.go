package stream_test

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

func TestHub_AFullQueueClosesTheConnectionWithResync(t *testing.T) {
	hub, _ := projectsHub(map[string]string{})
	conn := newFakeConn()
	conn.throttle = true
	done := served(t, hub, conn)
	conn.allow <- struct{}{} // the synced frame, then the writer holds still
	conn.sub(t, "projects")

	// The writer may hold one popped frame, so the bound is passed at QueueSize+2.
	for i := range stream.QueueSize + 2 {
		hub.Entity(stream.Projects, stream.KindOperation, fmt.Sprint(i), i, 0)
	}

	ended(t, done)
	assert.Equal(t, stream.CloseResync, conn.closeCode())
}

// rowsOf is a source of n rows.
func rowsOf(n int) *entities {
	rows := map[string]string{}
	for i := range n {
		rows[fmt.Sprint(i)] = "x"
	}
	return newEntities(rows)
}

// held subs scope while the writer holds still, so every frame waits in the queue.
func heldSub(t *testing.T, conn *fakeConn, scope string) {
	t.Helper()
	conn.send(t, stream.OpSub, scope)
	conn.send(t, stream.OpUnsub, "chat:barrier") // requests run in order: the sub is queued
}

func TestHub_ASnapshotLargerThanTheQueueIsDeliveredWhole(t *testing.T) {
	rows := 5 * stream.QueueSize
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindProjects: rowsOf(rows)}, nil)
	conn := newFakeConn()
	conn.throttle = true
	conn.allow = make(chan struct{}, rows+2) // the tokens never wait on the writer
	served(t, hub, conn)

	heldSub(t, conn, "projects")
	for range rows + 1 {
		conn.allow <- struct{}{}
	}
	seen := conn.until(t, syncedOn("projects"))

	require.Len(t, seen, rows+1)
	assert.Equal(t, 0, conn.closeCode(), "a snapshot never counts toward the live bound")
}

// A client that stops reading is closed at the hard bound instead of growing forever.
func TestHub_AStuckClientIsClosedAtTheHardBound(t *testing.T) {
	src := rowsOf(stream.HardQueueSize / 3)
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindChat: src}, nil)
	conn := newFakeConn()
	conn.throttle = true
	done := served(t, hub, conn)

	for i := range 4 { // the bound may close it before the last sub is read
		select {
		case conn.in <- stream.Request{Op: stream.OpSub, Scope: fmt.Sprintf("chat:c%d", i)}:
		case <-conn.closed:
		}
	}

	ended(t, done)
	assert.Equal(t, stream.CloseResync, conn.closeCode())
}

func TestHub_AResubWhileItsSnapshotIsPendingIsCoalesced(t *testing.T) {
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindProjects: rowsOf(3)}, nil)
	conn := newFakeConn()
	conn.throttle = true
	served(t, hub, conn)

	heldSub(t, conn, "projects")
	heldSub(t, conn, "projects")
	for range 3 * stream.QueueSize {
		conn.allow <- struct{}{}
	}
	hub.Entity(stream.Projects, stream.KindOperation, "marker", "", 0)
	seen := conn.until(t, func(f stream.Frame) bool { return f.ID == "marker" })

	assert.Len(t, seen, 3+1+1, "one snapshot, its synced, the marker")
}

// Every scope a client subs, lets go of and subs again before its first
// snapshot is written stays held: its resources keep running and live frames reach it.
func TestHub_ASubUnsubSubBeforeTheWriterDrainsKeepsTheScope(t *testing.T) {
	for _, name := range []string{"projects", "chat:c1", "ws:w1", "git:w1", "files:w1", "lsp:c1", "doc:/tmp/a.txt"} {
		t.Run(name, func(t *testing.T) {
			res := newRecorder()
			sources := map[stream.ScopeKind]appstream.Source{}
			resources := map[stream.ScopeKind][]appstream.Resource{}
			for _, k := range []stream.ScopeKind{stream.KindProjects, stream.KindChat, stream.KindWs} {
				sources[k], resources[k] = rowsOf(2), []appstream.Resource{res}
			}
			for _, k := range []stream.ScopeKind{stream.KindGit, stream.KindFiles, stream.KindLsp, stream.KindDoc} {
				sources[k], resources[k] = appstream.Unordered(rowsOf(2)), []appstream.Resource{res}
			}
			hub := newHub(sources, resources)
			conn := newFakeConn()
			conn.throttle = true
			served(t, hub, conn)
			scope, err := stream.ParseScope(name)
			require.NoError(t, err)

			heldSub(t, conn, name)
			conn.send(t, stream.OpUnsub, name)
			heldSub(t, conn, name)
			for range 2 * stream.QueueSize {
				conn.allow <- struct{}{}
			}
			hub.Signal(scope, "probe", 1)
			conn.until(t, func(f stream.Frame) bool { return f.Kind == "probe" && f.Scope == name })

			starts, stops := res.counts(scope.Key)
			assert.Equal(t, []int{2, 1}, []int{starts, stops}, "the second sub holds the resources again")
		})
	}
}

// Holding and letting go of scopes leaks no goroutine per cycle.
func TestHub_SubUnsubCyclesLeaveNoGoroutineBehind(t *testing.T) {
	res := newRecorder()
	hub := newHub(
		map[stream.ScopeKind]appstream.Source{stream.KindGit: appstream.Unordered(rowsOf(1)), stream.KindChat: rowsOf(1)},
		map[stream.ScopeKind][]appstream.Resource{stream.KindGit: {res}, stream.KindChat: {res}},
	)
	conn := newFakeConn()
	served(t, hub, conn)
	conn.sub(t, "git:w0")
	before := runtime.NumGoroutine()

	for i := range 100 {
		conn.sub(t, fmt.Sprintf("git:w%d", i%7))
		conn.sub(t, fmt.Sprintf("chat:c%d", i%5))
		conn.send(t, stream.OpUnsub, fmt.Sprintf("git:w%d", i%7))
		conn.send(t, stream.OpUnsub, fmt.Sprintf("chat:c%d", i%5))
	}
	conn.sub(t, "git:w0")

	// Eventually checks from a goroutine of its own.
	require.Eventually(t, func() bool { return runtime.NumGoroutine() <= before+1 },
		waitBound, time.Millisecond, "goroutines grew from %d", before)
}
