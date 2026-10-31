package stream_test

import (
	"fmt"
	"testing"

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
		hub.Entity(stream.Projects, stream.KindOperation, fmt.Sprint(i), i)
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

	for i := range 4 {
		conn.send(t, stream.OpSub, fmt.Sprintf("chat:c%d", i))
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
	hub.Entity(stream.Projects, stream.KindOperation, "marker", "")
	seen := conn.until(t, func(f stream.Frame) bool { return f.ID == "marker" })

	assert.Len(t, seen, 3+1+1, "one snapshot, its synced, the marker")
}
