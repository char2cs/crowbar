package stream_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
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
