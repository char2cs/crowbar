package stream_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// The read runs with the scope already live: a change that lands meanwhile
// reaches the client, and the read's older value of it is left out.
func TestHub_AChangeDuringASnapshotReadOutranksIt(t *testing.T) {
	side := &sidebar{
		entities: []appstream.Entity{{Kind: stream.KindChatEntity, ID: "c1", Data: "read"}, {Kind: stream.KindChatEntity, ID: "c2", Data: "kept"}},
		entered:  make(chan struct{}), gate: make(chan struct{}),
	}
	hub := newHub(map[stream.ScopeKind]appstream.Source{
		stream.KindProjects: appstream.NewProjectsSource(fixedOperations{}, side, encodeOperation),
	}, nil)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "projects")
	<-side.entered
	live := hub.Entity(stream.Projects, stream.KindChatEntity, "c1", "live", 0)
	close(side.gate)
	frames := conn.until(t, syncedOn("projects"))

	require.Len(t, frames, 3)
	assert.Equal(t, `"live"`, string(frames[0].Data), "published while the read was in flight")
	assert.Equal(t, live, frames[0].V)
	assert.Equal(t, "c2", frames[1].ID, "c1's read value is older than its live frame")
	assert.Greater(t, frames[1].V, uint64(0))
}

// A scope let go of while its snapshot reads is not answered by that read.
func TestHub_ASnapshotReadLetGoOfIsDropped(t *testing.T) {
	side := &sidebar{
		entities: []appstream.Entity{{Kind: stream.KindChatEntity, ID: "c1", Data: "read"}},
		entered:  make(chan struct{}), gate: make(chan struct{}),
	}
	hub := newHub(map[stream.ScopeKind]appstream.Source{
		stream.KindProjects: appstream.NewProjectsSource(fixedOperations{}, side, encodeOperation),
		stream.KindChat:     newEntities(map[string]string{}),
	}, nil)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "projects")
	<-side.entered
	conn.send(t, stream.OpUnsub, "projects")
	conn.sub(t, "chat:c9")
	close(side.gate)
	hub.AwaitReads()
	frames := conn.sub(t, "chat:c8")

	assert.Empty(t, frames, "nothing of the let-go projects read reached the client")
}
