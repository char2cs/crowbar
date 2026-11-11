package stream_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

func docHub(res *recorder) *appstream.Hub {
	src := appstream.NewDocSource(func(_ context.Context, path string) error {
		if path != "/h/notes.md" {
			return problem.New(problem.Forbidden, "not a host document")
		}
		return nil
	})
	return newHub(
		map[stream.ScopeKind]appstream.Source{stream.KindDoc: src},
		map[stream.ScopeKind][]appstream.Resource{stream.KindDoc: {res}},
	)
}

func TestDocSource_AnAdmittedPathIsWatchedAndSnapshotsNothing(t *testing.T) {
	res := newRecorder()
	conn := newFakeConn()
	served(t, docHub(res), conn)

	assert.Empty(t, conn.sub(t, "doc:/h/notes.md"))
	assert.Equal(t, []string{"start /h/notes.md"}, res.history())
}

func TestDocSource_ARefusedPathArmsNoWatch(t *testing.T) {
	res := newRecorder()
	conn := newFakeConn()
	served(t, docHub(res), conn)

	conn.send(t, stream.OpSub, "doc:/etc/shadow")
	closed := conn.next(t)

	assert.Equal(t, stream.ReasonRefused, closed.Reason)
	assert.Equal(t, problem.Forbidden, closed.Problem.Code)
	assert.Empty(t, res.history())
}
