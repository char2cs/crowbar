package stream_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

var chatC1 = stream.Scope{Kind: stream.KindChat, Key: "c1"}

func projectsHub(rows map[string]string) (*appstream.Hub, *entities) {
	src := newEntities(rows)
	return newHub(map[stream.ScopeKind]appstream.Source{stream.KindProjects: src}, nil), src
}

func TestHub_FirstFrameIsReadyWithTheEpoch(t *testing.T) {
	hub, _ := projectsHub(map[string]string{})
	conn := newFakeConn()
	go func() { _ = hub.Serve(t.Context(), conn) }()
	t.Cleanup(func() { _ = conn.CloseWith(1000, "") })

	assert.Equal(t, stream.Ready("epoch-1"), conn.next(t))
}

func TestHub_SnapshotThenSyncedThenLiveFrames(t *testing.T) {
	hub, _ := projectsHub(map[string]string{"op1": "accepted"})
	conn := newFakeConn()
	served(t, hub, conn)

	snapshot := conn.sub(t, "projects")
	require.Len(t, snapshot, 1)
	assert.Equal(t, stream.FrameEntity, snapshot[0].T)
	assert.Equal(t, "projects", snapshot[0].Scope)
	assert.Equal(t, "op1", snapshot[0].ID)
	assert.JSONEq(t, `"accepted"`, string(snapshot[0].Data))

	v := hub.Entity(stream.Projects, stream.KindOperation, "op1", "running")
	live := conn.next(t)
	assert.Equal(t, stream.Frame{
		T: stream.FrameEntity, Scope: "projects", Kind: "operation", ID: "op1", V: v, Data: []byte(`"running"`),
	}, live)
	assert.Greater(t, live.V, snapshot[0].V)
}

// Writers save then publish, as the operation service does; a sub snapshots
// meanwhile. For each id the client must see v rise across snapshot and live.
func TestHub_LiveFramesNeverPrecedeOrUndercutTheSnapshot(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e"}
	hub, src := projectsHub(map[string]string{"a": "0", "b": "0", "c": "0", "d": "0", "e": "0"})
	conn := newFakeConn()
	served(t, hub, conn)

	var writers sync.WaitGroup
	for i := range 50 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			id := ids[i%len(ids)]
			value := fmt.Sprint(i)
			src.set(id, value)
			hub.Entity(stream.Projects, stream.KindOperation, id, value)
		}()
	}
	snapshot := conn.sub(t, "projects")
	writers.Wait()
	marker := hub.Entity(stream.Projects, stream.KindOperation, "marker", "")
	live := conn.until(t, func(f stream.Frame) bool { return f.ID == "marker" })

	require.Len(t, snapshot, len(ids))
	last := map[string]uint64{}
	for _, f := range snapshot {
		assert.Equal(t, snapshot[0].V, f.V, "a snapshot is stamped at one version")
		last[f.ID] = f.V
	}
	for _, f := range live {
		assert.Greater(t, f.V, last[f.ID], "id %s", f.ID)
		last[f.ID] = f.V
	}
	assert.Equal(t, marker, last["marker"])
}

func TestHub_AnEntityFrameReachesOnlyConnectionsHoldingItsScope(t *testing.T) {
	src := newEntities(map[string]string{})
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindProjects: src, stream.KindChat: src}, nil)
	projects, chat := newFakeConn(), newFakeConn()
	served(t, hub, projects)
	served(t, hub, chat)
	projects.sub(t, "projects")
	chat.sub(t, "chat:c1")

	hub.Entity(stream.Projects, stream.KindOperation, "op1", "x")
	hub.Entity(chatC1, stream.KindOperation, "op2", "y")

	assert.Equal(t, "op1", projects.next(t).ID)
	assert.Equal(t, "op2", chat.next(t).ID, "the projects frame never reached the chat holder")
}

func TestHub_EntityReturnsTheVersionItStamped(t *testing.T) {
	hub, _ := projectsHub(map[string]string{})
	conn := newFakeConn()
	served(t, hub, conn)
	conn.sub(t, "projects")

	first := hub.Entity(stream.Projects, stream.KindOperation, "op1", "a")
	second := hub.Entity(stream.Projects, stream.KindOperation, "op1", "b")

	assert.Equal(t, first, conn.next(t).V)
	assert.Equal(t, second, conn.next(t).V)
	assert.Greater(t, second, first)
}

func TestHub_AnUnknownScopeIsClosedRefusedAndTheConnectionLives(t *testing.T) {
	hub, _ := projectsHub(map[string]string{"op1": "x"})
	conn := newFakeConn()
	served(t, hub, conn)

	for _, scope := range []string{"nope:x", "git:w1"} {
		conn.send(t, stream.OpSub, scope)
		closed := conn.next(t)
		assert.Equal(t, stream.FrameClosed, closed.T, scope)
		assert.Equal(t, scope, closed.Scope)
		assert.Equal(t, stream.ReasonRefused, closed.Reason)
		require.NotNil(t, closed.Problem, scope)
		assert.Equal(t, problem.InvalidArgument, closed.Problem.Code)
	}
	assert.Len(t, conn.sub(t, "projects"), 1, "the connection still serves")
}

func TestHub_UnsubStopsFramesForThatScope(t *testing.T) {
	src := newEntities(map[string]string{})
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindProjects: src, stream.KindChat: src}, nil)
	conn := newFakeConn()
	served(t, hub, conn)
	conn.sub(t, "projects")
	conn.send(t, stream.OpUnsub, "projects")
	conn.sub(t, "chat:c1") // requests are handled in order: the unsub has landed

	hub.Entity(stream.Projects, stream.KindOperation, "op1", "x")
	hub.Entity(chatC1, stream.KindOperation, "op2", "y")

	assert.Equal(t, "op2", conn.next(t).ID)
}

func TestHub_ReturningReleasesEveryHeldScope(t *testing.T) {
	src := newEntities(map[string]string{})
	res := newRecorder()
	hub := newHub(
		map[stream.ScopeKind]appstream.Source{stream.KindChat: src},
		map[stream.ScopeKind][]appstream.Resource{stream.KindChat: {res}},
	)
	conn := newFakeConn()
	done := served(t, hub, conn)
	conn.sub(t, "chat:c1")
	conn.sub(t, "chat:c2")

	require.NoError(t, conn.CloseWith(1000, ""))
	ended(t, done)

	for _, key := range []string{"c1", "c2"} {
		starts, stops := res.counts(key)
		assert.Equal(t, 1, starts, key)
		assert.Equal(t, 1, stops, key)
	}
}

func TestHub_CloseEndsEveryConnection(t *testing.T) {
	hub, _ := projectsHub(map[string]string{})
	conn := newFakeConn()
	done := served(t, hub, conn)

	hub.Close()
	ended(t, done)

	assert.Equal(t, uint64(0), hub.Entity(stream.Projects, stream.KindOperation, "op1", "x"), "a closed hub stamps nothing")
}
