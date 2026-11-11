package stream_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// presenceHub serves projects as the daemon does, a Reader, and chats.
func presenceHub() *appstream.Hub {
	return newHub(map[stream.ScopeKind]appstream.Source{
		stream.KindProjects: appstream.NewProjectsSource(fixedOperations{}, &sidebar{}, encodeOperation),
		stream.KindChat:     newEntities(map[string]string{}),
	}, nil)
}

func isPresence(f stream.Frame) bool { return f.Kind == string(stream.KindPresence) }

// viewersIn is a presence frame's count, -1 for gone.
func viewersIn(t *testing.T, f stream.Frame) int {
	t.Helper()
	require.True(t, isPresence(f), "a presence frame, got %+v", f)
	if f.Gone {
		return -1
	}
	var p appstream.Presence
	require.NoError(t, json.Unmarshal(f.Data, &p))
	return p.Viewers
}

// watching is a connection holding projects, past its snapshot.
func watching(t *testing.T, hub *appstream.Hub) *fakeConn {
	t.Helper()
	conn := newFakeConn()
	served(t, hub, conn)
	conn.sub(t, "projects")
	return conn
}

// viewer is a connection holding chat:id.
func viewer(t *testing.T, hub *appstream.Hub, id string) *fakeConn {
	t.Helper()
	conn := newFakeConn()
	served(t, hub, conn)
	conn.sub(t, "chat:"+id)
	return conn
}

func TestPresence_TwoConnectionsHoldingAChatCountTwo(t *testing.T) {
	hub := presenceHub()
	watcher := watching(t, hub)

	viewer(t, hub, "c1")
	first := watcher.next(t)
	viewer(t, hub, "c1")
	second := watcher.next(t)

	assert.Equal(t, "projects", first.Scope)
	assert.Equal(t, "c1", first.ID)
	assert.Equal(t, 1, viewersIn(t, first))
	assert.Equal(t, 2, viewersIn(t, second))
	assert.Greater(t, second.V, first.V)
}

func TestPresence_TheLastReleaseSendsGone(t *testing.T) {
	hub := presenceHub()
	watcher := watching(t, hub)
	a, b := viewer(t, hub, "c1"), viewer(t, hub, "c1")
	watcher.next(t)
	watcher.next(t)

	a.send(t, stream.OpUnsub, "chat:c1")
	assert.Equal(t, 1, viewersIn(t, watcher.next(t)))
	b.send(t, stream.OpUnsub, "chat:c1")
	gone := watcher.next(t)

	assert.Equal(t, -1, viewersIn(t, gone))
	assert.Equal(t, "c1", gone.ID)
	viewer(t, hub, "c1")
	assert.Equal(t, 1, viewersIn(t, watcher.next(t)), "a chat viewed again after gone counts again")
}

func TestPresence_ADroppedConnectionCountsAsALeave(t *testing.T) {
	hub := presenceHub()
	watcher := watching(t, hub)
	conn := newFakeConn()
	done := served(t, hub, conn)
	conn.sub(t, "chat:c1")
	watcher.next(t)

	require.NoError(t, conn.CloseWith(1000, ""))
	ended(t, done)

	assert.Equal(t, -1, viewersIn(t, watcher.next(t)))
}

func TestPresence_AProjectsSnapshotCarriesTheCurrentCounts(t *testing.T) {
	hub := presenceHub()
	viewer(t, hub, "c1")
	viewer(t, hub, "c1")
	viewer(t, hub, "c2")
	conn := newFakeConn()
	served(t, hub, conn)

	counts := map[string]int{}
	for _, f := range conn.sub(t, "projects") {
		counts[f.ID] = viewersIn(t, f)
	}

	assert.Equal(t, map[string]int{"c1": 2, "c2": 1}, counts)
}

func TestPresence_ResubscribingTheSameConnectionDoesNotCountTwice(t *testing.T) {
	hub := presenceHub()
	watcher := watching(t, hub)
	a := viewer(t, hub, "c1")
	assert.Equal(t, 1, viewersIn(t, watcher.next(t)))

	a.sub(t, "chat:c1")
	viewer(t, hub, "c1")

	assert.Equal(t, 2, viewersIn(t, watcher.next(t)), "the resub sent nothing; the second viewer counts 2")
}

// Viewers come and go while a projects sub snapshots: per chat, v never falls
// across snapshot and live, and the last frame says what holds now.
func TestPresence_VersionsIncreaseAcrossSnapshotAndLive(t *testing.T) {
	hub := presenceHub()
	viewers := make([]*fakeConn, 5)
	for i := range viewers {
		viewers[i] = newFakeConn()
		served(t, hub, viewers[i])
	}
	watcher := newFakeConn()
	served(t, hub, watcher)

	var churn sync.WaitGroup
	for i, conn := range viewers {
		churn.Add(1)
		go func() {
			defer churn.Done()
			for n := range 10 {
				scope := fmt.Sprintf("chat:c%d", (i+n)%3)
				conn.in <- stream.Request{Op: stream.OpSub, Scope: scope}
				conn.in <- stream.Request{Op: stream.OpUnsub, Scope: scope}
			}
			// Requests run in order: once this is read, the last unsub has landed.
			conn.in <- stream.Request{Op: stream.OpUnsub, Scope: "chat:barrier"}
		}()
	}
	snapshot := watcher.sub(t, "projects")
	churn.Wait()
	marker := hub.Entity(stream.Projects, stream.KindOperation, "marker", "", 0)
	live := watcher.until(t, func(f stream.Frame) bool { return f.ID == "marker" })

	last, now := map[string]uint64{}, map[string]int{}
	for _, f := range append(snapshot, live...) {
		if !isPresence(f) {
			continue
		}
		assert.GreaterOrEqual(t, f.V, last[f.ID], "chat %s", f.ID)
		last[f.ID], now[f.ID] = f.V, viewersIn(t, f)
	}
	for id, n := range now {
		assert.Equal(t, -1, n, "nobody views %s once the churn ends", id)
		assert.Less(t, last[id], marker)
	}
}
