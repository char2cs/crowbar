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
	hub.Entity(stream.Projects, stream.KindOperation, "op1", "live")
	assert.Equal(t, "op1", other.next(t).ID)

	close(res.gate)
	slow.until(t, syncedOn("git:w1"))
}

func TestRefcount_AStartWaitsForTheStopBeforeIt(t *testing.T) {
	res := newRecorder()
	hub := gitHubOver(res)
	first, second := newFakeConn(), newFakeConn()
	served(t, hub, first)
	served(t, hub, second)
	first.sub(t, "git:w1")

	res.gated = true
	first.send(t, stream.OpUnsub, "git:w1")
	res.enteredAs(t, "stop w1")
	second.send(t, stream.OpSub, "git:w1")
	res.gate <- struct{}{}
	res.enteredAs(t, "start w1")
	res.gate <- struct{}{}
	second.until(t, syncedOn("git:w1"))

	assert.Equal(t, []string{"start w1", "stop w1", "start w1"}, res.history())
	close(res.gate) // the cleanup's own stop passes
}
