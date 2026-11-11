package stream_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

func TestRefcount_StartsOnFirstHoldAndStopsOnLast(t *testing.T) {
	res := newRecorder()
	hub := newHub(
		map[stream.ScopeKind]appstream.Source{stream.KindGit: newEntities(map[string]string{})},
		map[stream.ScopeKind][]appstream.Resource{stream.KindGit: {res}},
	)
	first, second := newFakeConn(), newFakeConn()
	served(t, hub, first)
	served(t, hub, second)

	first.sub(t, "git:w1")
	second.sub(t, "git:w1")
	first.sub(t, "git:w1") // a re-sub is not a second hold
	first.send(t, stream.OpUnsub, "git:w1")
	first.sub(t, "git:w2") // the unsub has landed
	starts, stops := res.counts("w1")
	assert.Equal(t, 1, starts)
	assert.Equal(t, 0, stops, "the second connection still holds it")

	second.send(t, stream.OpUnsub, "git:w1")
	second.sub(t, "git:w2")
	starts, stops = res.counts("w1")
	assert.Equal(t, 1, starts)
	assert.Equal(t, 1, stops)
}

func TestRefcount_AResourceUnderTwoKindsStartsOnce(t *testing.T) {
	watcher := newRecorder()
	src := newEntities(map[string]string{})
	hub := newHub(
		map[stream.ScopeKind]appstream.Source{stream.KindGit: src, stream.KindFiles: src, stream.KindProjects: src},
		map[stream.ScopeKind][]appstream.Resource{stream.KindGit: {watcher}, stream.KindFiles: {watcher}},
	)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.sub(t, "git:w1")
	conn.sub(t, "files:w1")
	conn.send(t, stream.OpUnsub, "git:w1")
	conn.sub(t, "projects")
	starts, stops := watcher.counts("w1")
	assert.Equal(t, 1, starts)
	assert.Equal(t, 0, stops)

	conn.send(t, stream.OpUnsub, "files:w1")
	conn.send(t, stream.OpUnsub, "projects")
	conn.sub(t, "projects")
	starts, stops = watcher.counts("w1")
	assert.Equal(t, 1, starts)
	assert.Equal(t, 1, stops)
}

func TestRefcount_AScopeWithoutResourcesIsFree(t *testing.T) {
	res := newRecorder()
	hub := newHub(
		map[stream.ScopeKind]appstream.Source{stream.KindProjects: newEntities(map[string]string{})},
		map[stream.ScopeKind][]appstream.Resource{stream.KindGit: {res}},
	)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.sub(t, "projects")
	conn.send(t, stream.OpUnsub, "projects")
	conn.sub(t, "projects")

	starts, stops := res.counts("")
	assert.Equal(t, 0, starts)
	assert.Equal(t, 0, stops)
}

func gitHubOver(res appstream.Resource) *appstream.Hub {
	src := newEntities(map[string]string{})
	return newHub(
		map[stream.ScopeKind]appstream.Source{stream.KindGit: src, stream.KindProjects: src},
		map[stream.ScopeKind][]appstream.Resource{stream.KindGit: {res}},
	)
}

func TestRefcount_AFailedStartRefusesTheScopeWithItsProblem(t *testing.T) {
	res := newRecorder()
	res.fail["w1"] = problem.New(problem.WatchLimit, "no watches left")
	hub := gitHubOver(res)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "git:w1")
	closed := conn.next(t)
	assert.Equal(t, stream.FrameClosed, closed.T)
	assert.Equal(t, stream.ReasonRefused, closed.Reason)
	require.NotNil(t, closed.Problem)
	assert.Equal(t, problem.WatchLimit, closed.Problem.Code)

	delete(res.fail, "w1")
	conn.sub(t, "git:w1") // a later sub tries again, and the connection lived
	starts, stops := res.counts("w1")
	assert.Equal(t, 2, starts)
	assert.Equal(t, 0, stops, "what never started is never stopped")
}

// Start may arm a watch or read a workspace: other connections' frames must not wait on it.
func TestRefcount_StartRunsOutsideTheHubLock(t *testing.T) {
	res := newRecorder()
	res.gated = true
	hub := gitHubOver(res)
	slow, other := newFakeConn(), newFakeConn()
	served(t, hub, slow)
	served(t, hub, other)

	slow.send(t, stream.OpSub, "git:w1")
	res.enteredAs(t, "start w1")
	other.sub(t, "projects")
	hub.Entity(stream.Projects, stream.KindOperation, "op1", "live", 0)
	assert.Equal(t, "op1", other.next(t).ID)

	close(res.gate)
	slow.until(t, syncedOn("git:w1"))
}

// A Start of a key never runs while that key's Stop does: repeated, so a
// Start that did not wait would enter during one of the held Stops.
func TestRefcount_AStartWaitsForTheStopBeforeIt(t *testing.T) {
	res := newRecorder()
	res.stopGate = make(chan struct{})
	hub := gitHubOver(res)
	first, second := newFakeConn(), newFakeConn()
	served(t, hub, first)
	served(t, hub, second)

	for range 32 {
		first.sub(t, "git:w1")
		first.send(t, stream.OpUnsub, "git:w1")
		res.enteredAs(t, "stop w1")
		second.send(t, stream.OpSub, "git:w1")
		res.stopGate <- struct{}{}
		second.until(t, syncedOn("git:w1"))
		second.send(t, stream.OpUnsub, "git:w1")
		res.enteredAs(t, "stop w1")
		res.stopGate <- struct{}{}
	}

	res.mu.Lock()
	defer res.mu.Unlock()
	assert.Zero(t, res.overlaps, "a Start ran while its key's Stop did")
	assert.Equal(t, 64, res.starts["w1"])
}

// A scope naming two resources whose second fails to start stops the first:
// the refused scope holds nothing.
func TestRefcount_AFailedLaterStartStopsTheEarlierOnes(t *testing.T) {
	first, second := newRecorder(), newRecorder()
	second.fail["w1"] = problem.New(problem.Unavailable, "no")
	src := newEntities(map[string]string{})
	hub := newHub(
		map[stream.ScopeKind]appstream.Source{stream.KindGit: src, stream.KindProjects: src},
		map[stream.ScopeKind][]appstream.Resource{stream.KindGit: {first, second}},
	)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "git:w1")
	assert.Equal(t, stream.ReasonRefused, conn.next(t).Reason)
	conn.sub(t, "projects")

	starts, stops := first.counts("w1")
	assert.Equal(t, []int{1, 1}, []int{starts, stops}, "the first resource was rolled back")
}
