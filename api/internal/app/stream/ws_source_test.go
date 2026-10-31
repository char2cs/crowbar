package stream_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type knownWorkspaces map[string]domain.Workspace

func (k knownWorkspaces) Get(_ context.Context, id string) (domain.Workspace, error) {
	ws, ok := k[id]
	if !ok {
		return domain.Workspace{}, problem.ErrNotFound
	}
	return ws, nil
}

type threadsByWorkspace map[string][]domain.ReviewThread

func (t threadsByWorkspace) ListByWorkspace(_ context.Context, wsID string) ([]domain.ReviewThread, error) {
	return t[wsID], nil
}

// encodeThread stands in for the API's encoder: the thread's id and the scope ids it names.
func encodeThread(_ stream.EntityKind, v any) (json.RawMessage, error) {
	th, ok := v.(appstream.ReviewThread)
	if !ok {
		return nil, errors.New("not a review thread")
	}
	return json.Marshal([]string{th.Thread.ID, th.ProjectID, th.RepoID})
}

func TestWsSource_SnapshotsTheWorkspacesReviewThreads(t *testing.T) {
	src := appstream.NewWsSource(
		knownWorkspaces{"w1": {ID: "w1", ProjectID: "p1", RepoID: "r1"}, "w2": {ID: "w2"}},
		threadsByWorkspace{"w1": {{ID: "th1", WsID: "w1"}, {ID: "th2", WsID: "w1"}}, "w2": {{ID: "th3", WsID: "w2"}}},
		encodeThread,
	)
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindWs: src}, nil)
	conn := newFakeConn()
	served(t, hub, conn)

	frames := conn.sub(t, "ws:w1")

	require.Len(t, frames, 2)
	for i, id := range []string{"th1", "th2"} {
		assert.Equal(t, stream.FrameEntity, frames[i].T)
		assert.Equal(t, string(stream.KindReviewThread), frames[i].Kind)
		assert.Equal(t, id, frames[i].ID)
		assert.JSONEq(t, `["`+id+`","p1","r1"]`, string(frames[i].Data))
		assert.NotZero(t, frames[i].V)
	}
}

func TestWsSource_AnUnknownWorkspaceIsRefused(t *testing.T) {
	src := appstream.NewWsSource(knownWorkspaces{}, threadsByWorkspace{}, encodeThread)
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindWs: src}, nil)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "ws:gone")
	closed := conn.next(t)

	assert.Equal(t, stream.ReasonRefused, closed.Reason)
	assert.Equal(t, problem.NotFound, closed.Problem.Code)
}
