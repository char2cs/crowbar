package stream_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

func TestHub_TooManyScopesAreRefused(t *testing.T) {
	src := newEntities(map[string]string{})
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindChat: src}, nil)
	conn := newFakeConn()
	served(t, hub, conn)
	for i := range stream.MaxScopes {
		conn.sub(t, fmt.Sprintf("chat:c%d", i))
	}

	conn.send(t, stream.OpSub, "chat:one-more")
	closed := conn.next(t)

	assert.Equal(t, stream.FrameClosed, closed.T)
	assert.Equal(t, "chat:one-more", closed.Scope)
	assert.Equal(t, stream.ReasonRefused, closed.Reason)
}

// An overlong scope is that scope's refusal, so a holder waiting on it hears why.
func TestHub_AnOverlongScopeIsRefused(t *testing.T) {
	hub, _ := projectsHub(map[string]string{})
	conn := newFakeConn()
	served(t, hub, conn)

	long := "doc:/" + strings.Repeat("x", stream.MaxScopeLen)
	conn.send(t, stream.OpSub, long)
	got := conn.next(t)

	assert.Equal(t, stream.FrameClosed, got.T)
	assert.Equal(t, long, got.Scope)
	assert.Equal(t, stream.ReasonRefused, got.Reason)
	require.NotNil(t, got.Problem)
	assert.Equal(t, problem.InvalidArgument, got.Problem.Code)
}

// An op the hub does not know is the connection's error, never a held scope's.
func TestHub_AnUnknownOpIsAConnectionLevelError(t *testing.T) {
	hub, _ := projectsHub(map[string]string{})
	conn := newFakeConn()
	served(t, hub, conn)
	conn.sub(t, "projects")

	conn.send(t, "gossip", "projects")
	got := conn.next(t)
	assert.Equal(t, stream.FrameError, got.T)
	assert.Empty(t, got.Scope)

	hub.Entity(stream.Projects, stream.KindOperation, "op1", "x")
	assert.Equal(t, "op1", conn.next(t).ID, "projects is still held")
}

func TestHub_AnUndecodableMessageClosesWithUnsupportedData(t *testing.T) {
	hub, _ := projectsHub(map[string]string{})
	conn := newFakeConn()
	done := served(t, hub, conn)

	conn.bad <- &json.SyntaxError{}

	ended(t, done)
	ended(t, conn.closed) // the close is sent from aside
	assert.Equal(t, 1003, conn.closeCode())
}

// gate is a source whose snapshot, once entered, waits until a live publish is on its way.
type gate struct{ entered, encoding chan struct{} }

func (g gate) Attach(_ context.Context, _ string, _ int64, emit func(stream.Frame)) error {
	close(g.entered)
	<-g.encoding
	emit(stream.Frame{T: stream.FrameEntity, Kind: string(stream.KindOperation), ID: "op1", Data: []byte(`"snap"`)})
	return nil
}

func TestHub_ALiveFrameRacingASnapshotFollowsIt(t *testing.T) {
	g := gate{entered: make(chan struct{}), encoding: make(chan struct{})}
	var once sync.Once
	encode := func(k stream.EntityKind, v any) (json.RawMessage, error) {
		once.Do(func() { close(g.encoding) })
		return encodeJSON(k, v)
	}
	hub := appstream.New("e", map[stream.ScopeKind]appstream.Source{stream.KindProjects: g}, nil, encode)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "projects")
	<-g.entered
	published := make(chan uint64)
	go func() { published <- hub.Entity(stream.Projects, stream.KindOperation, "op1", "live") }()
	frames := conn.until(t, func(f stream.Frame) bool { return f.ID == "op1" && string(f.Data) == `"live"` })
	v := <-published

	require.Len(t, frames, 3, "snapshot, synced, live")
	assert.Equal(t, `"snap"`, string(frames[0].Data))
	assert.Equal(t, stream.FrameSynced, frames[1].T)
	assert.Greater(t, v, frames[0].V)
}

func TestHub_ASignalReachesOnlyTheConnectionsHoldingItsScope(t *testing.T) {
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindGit: newEntities(map[string]string{})}, nil)
	holder, other := newFakeConn(), newFakeConn()
	served(t, hub, holder)
	served(t, hub, other)
	holder.sub(t, "git:w1")
	other.sub(t, "git:w2")

	hub.Signal(stream.Scope{Kind: stream.KindGit, Key: "w1"}, "git_status", map[string]string{"branch": "main"})
	hub.Signal(stream.Scope{Kind: stream.KindGit, Key: "w2"}, "git_status", "w2")

	assert.Equal(t, stream.Frame{
		T: stream.FrameSignal, Scope: "git:w1", Kind: "git_status", Data: []byte(`{"branch":"main"}`),
	}, holder.next(t))
	assert.Equal(t, `"w2"`, string(other.next(t).Data), "w1's signal never reached w2's holder")
}

func TestHub_GoneIsAVersionedEntityFrameWithoutData(t *testing.T) {
	hub, _ := projectsHub(map[string]string{})
	conn := newFakeConn()
	served(t, hub, conn)
	conn.sub(t, "projects")

	v := hub.Entity(stream.Projects, stream.KindOperation, "op1", "running")
	hub.Gone(stream.Projects, stream.KindOperation, "op1")

	conn.next(t)
	gone := conn.next(t)
	assert.Equal(t, stream.Frame{T: stream.FrameEntity, Scope: "projects", Kind: "operation", ID: "op1", V: v + 1, Gone: true}, gone)
}

// held is a signal source whose snapshot, once entered, waits to be let go.
type held struct{ entered, release chan struct{} }

func (s held) Attach(_ context.Context, _ string, _ int64, emit func(stream.Frame)) error {
	close(s.entered)
	<-s.release
	emit(stream.Frame{T: stream.FrameSignal, Kind: "git_status", Data: []byte(`"snap"`)})
	return nil
}

func TestHub_AnUnorderedSourceSnapshotsAfterGoingLiveAndOutsideTheLock(t *testing.T) {
	src := held{entered: make(chan struct{}), release: make(chan struct{})}
	hub := newHub(map[stream.ScopeKind]appstream.Source{
		stream.KindGit: appstream.Unordered(src), stream.KindProjects: newEntities(map[string]string{}),
	}, nil)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "git:w1")
	ended(t, src.entered)
	hub.Entity(stream.Projects, stream.KindOperation, "op1", "free") // the lock is not held
	hub.Signal(stream.Scope{Kind: stream.KindGit, Key: "w1"}, "git_status", "live")
	close(src.release)

	frames := conn.until(t, syncedOn("git:w1"))
	require.Len(t, frames, 3)
	assert.Equal(t, `"live"`, string(frames[0].Data))
	assert.Equal(t, `"snap"`, string(frames[1].Data))
	assert.Equal(t, "git:w1", frames[1].Scope)
}

// absent answers every snapshot as something that is not there.
type absent struct{}

func (absent) Attach(context.Context, string, int64, func(stream.Frame)) error {
	return fmt.Errorf("ws source: %w", problem.New(problem.NotFound, "no workspace w9"))
}

func TestHub_ASnapshotOfSomethingAbsentIsRefusedAndReleasesItsResources(t *testing.T) {
	res := newRecorder()
	hub := newHub(
		map[stream.ScopeKind]appstream.Source{stream.KindWs: absent{}, stream.KindProjects: newEntities(map[string]string{})},
		map[stream.ScopeKind][]appstream.Resource{stream.KindWs: {res}},
	)
	conn := newFakeConn()
	served(t, hub, conn)

	for range 2 { // refused each time, never coalesced into a snapshot still pending
		conn.send(t, stream.OpSub, "ws:w9")
		closed := conn.next(t)
		require.Equal(t, stream.FrameClosed, closed.T)
		assert.Equal(t, stream.ReasonRefused, closed.Reason)
		assert.Equal(t, problem.NotFound, closed.Problem.Code)
	}
	conn.sub(t, "projects")
	starts, stops := res.counts("w9")
	assert.Equal(t, 2, starts)
	assert.Equal(t, 2, stops)
}

// A source that went live before its refused snapshot must not keep the
// refused scope registered: nothing published to it reaches the client.
func TestHub_ARefusedLiveSourceHearsNothingMore(t *testing.T) {
	hub := newHub(map[stream.ScopeKind]appstream.Source{
		stream.KindGit:      appstream.Unordered(absent{}),
		stream.KindProjects: newEntities(map[string]string{}),
	}, nil)
	conn := newFakeConn()
	served(t, hub, conn)
	conn.send(t, stream.OpSub, "git:w9")
	require.Equal(t, stream.ReasonRefused, conn.next(t).Reason)
	conn.sub(t, "projects")

	hub.Signal(stream.Scope{Kind: stream.KindGit, Key: "w9"}, stream.SignalGitStatus, map[string]string{})
	hub.Entity(stream.Projects, stream.KindOperation, "marker", "")
	seen := conn.until(t, func(f stream.Frame) bool { return f.ID == "marker" })

	for _, f := range seen {
		assert.NotEqual(t, "git:w9", f.Scope, "a refused scope heard a signal")
	}
}
