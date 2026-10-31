package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	domainstream "github.com/char2cs/crowbar/api/internal/domain/stream"
)

// pipeConn is a stream client: requests go in, frames come out.
type pipeConn struct {
	in  chan domainstream.Request
	out chan domainstream.Frame
}

func (p pipeConn) ReadJSON(v any) error {
	req, ok := <-p.in
	if !ok {
		return io.EOF
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func (p pipeConn) WriteJSON(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var f domainstream.Frame
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	p.out <- f
	return nil
}

func (pipeConn) CloseWith(int, string) error { return nil }

// An agent's review comment must reach a review pane already open on its
// workspace, carrying the project and repo its wire form names.
func TestLateStream_AnAgentsReviewThreadReachesItsWorkspaceScope(t *testing.T) {
	encode := func(_ domainstream.EntityKind, v any) (json.RawMessage, error) {
		th, ok := v.(stream.ReviewThread)
		if !ok {
			return nil, errors.New("not a review thread")
		}
		return json.Marshal([]string{th.Thread.ID, th.ProjectID, th.RepoID})
	}
	late := &lateStream{}
	late.hub = stream.New("e1", map[domainstream.ScopeKind]stream.Source{domainstream.KindWs: stream.Quiet{}}, nil, encode)
	conn := pipeConn{in: make(chan domainstream.Request), out: make(chan domainstream.Frame, 8)}
	go func() { _ = late.hub.Serve(context.Background(), conn) }()
	t.Cleanup(func() { close(conn.in) })
	conn.in <- domainstream.Request{Op: domainstream.OpSub, Scope: "ws:w1"}
	for f := range conn.out {
		if f.T == domainstream.FrameSynced {
			break
		}
	}

	late.reviewThread()(domain.ReviewThread{ID: "t1", WsID: "w1"}, "p1", "r1")

	f := <-conn.out
	require.Equal(t, domainstream.FrameEntity, f.T)
	assert.Equal(t, "ws:w1", f.Scope)
	assert.Equal(t, string(domainstream.KindReviewThread), f.Kind)
	assert.Equal(t, "t1", f.ID)
	assert.JSONEq(t, `["t1","p1","r1"]`, string(f.Data))
}
