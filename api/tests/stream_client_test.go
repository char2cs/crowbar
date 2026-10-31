//go:build integration

package tests

import (
	"encoding/json"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// streamClient is one /v0/stream connection as a test drives it.
type streamClient struct {
	conn *websocket.Conn
}

func dialStream(t *testing.T, h *harness) *streamClient {
	t.Helper()
	return &streamClient{conn: h.dial("/v0/stream")}
}

func (c *streamClient) Sub(t *testing.T, scope string, since int64) {
	t.Helper()
	require.NoError(t, c.conn.WriteJSON(stream.Request{Op: stream.OpSub, Scope: scope, Since: since}))
}

func (c *streamClient) Unsub(t *testing.T, scope string) {
	t.Helper()
	require.NoError(t, c.conn.WriteJSON(stream.Request{Op: stream.OpUnsub, Scope: scope}))
}

// Next reads frames until match holds, under readUntil's failure bound.
func (c *streamClient) Next(t *testing.T, match func(stream.Frame) bool) stream.Frame {
	t.Helper()
	var got stream.Frame
	readUntil(t, c.conn, func(raw map[string]any) bool {
		encoded, err := json.Marshal(raw)
		require.NoError(t, err)
		got = stream.Frame{}
		require.NoError(t, json.Unmarshal(encoded, &got))
		return match(got)
	})
	return got
}

func operationIn(t *testing.T, f stream.Frame) domain.Operation {
	t.Helper()
	var op domain.Operation
	require.NoError(t, json.Unmarshal(f.Data, &op))
	return op
}

func isOperationFrame(f stream.Frame, id string) bool {
	return f.T == stream.FrameEntity && f.Kind == string(stream.KindOperation) && f.ID == id
}

// awaitOperation holds the projects scope and reads until done holds for
// operation id. The scope's snapshot carries the operation's current row, so a
// change that happened before the dial is never missed.
func awaitOperation(
	t *testing.T,
	h *harness,
	id string,
	done func(domain.Operation) bool,
) domain.Operation {
	t.Helper()
	c := dialStream(t, h)
	c.Sub(t, "projects", 0)
	var got domain.Operation
	c.Next(t, func(f stream.Frame) bool {
		if !isOperationFrame(f, id) {
			return false
		}
		got = operationIn(t, f)
		return done(got)
	})
	require.NoError(t, c.conn.Close())
	return got
}
