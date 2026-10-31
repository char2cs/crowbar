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

func TestHub_ASnapshotLargerThanTheQueueIsDeliveredWhole(t *testing.T) {
	rows := map[string]string{}
	for i := range 3 * stream.QueueSize {
		rows[fmt.Sprint(i)] = "x"
	}
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindProjects: newEntities(rows)}, nil)
	conn := newFakeConn()
	served(t, hub, conn)

	snapshot := conn.sub(t, "projects")

	require.Len(t, snapshot, 3*stream.QueueSize)
	assert.Equal(t, 0, conn.closeCode(), "a snapshot never counts toward the bound")
}
