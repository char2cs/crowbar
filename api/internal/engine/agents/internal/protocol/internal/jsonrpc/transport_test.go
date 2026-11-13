package jsonrpc_test

import (
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
)

// fakeTransport is message-delimited: no newline framing, one slice per message.
type fakeTransport struct {
	in      chan []byte // what the peer sends to the Conn
	out     chan []byte // what the Conn sent
	sendErr error
	done    chan struct{}
}

func newFake(t *testing.T) *fakeTransport {
	f := &fakeTransport{in: make(chan []byte, 8), out: make(chan []byte, 8), done: make(chan struct{})}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func (f *fakeTransport) Send(b []byte) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	f.out <- b
	return nil
}

func (f *fakeTransport) Recv() ([]byte, error) {
	select {
	case b, ok := <-f.in:
		if !ok {
			return nil, io.EOF
		}
		return b, nil
	case <-f.done:
		return nil, io.EOF
	}
}

func (f *fakeTransport) Close() error {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	return nil
}

func overTransport(t *testing.T, f *fakeTransport) *jsonrpc.Conn {
	conn := jsonrpc.NewTransport(f)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestTransport_CallRoundTripsFramesWithEmbeddedNewlines(t *testing.T) {
	f := newFake(t)
	conn := overTransport(t, f)
	answered := make(chan json.RawMessage, 1)
	go func() {
		result, err := conn.Call(t.Context(), "echo", map[string]string{"s": "a\nb"})
		assert.NoError(t, err)
		answered <- result
	}()

	var req struct {
		ID     json.RawMessage   `json:"id"`
		Params map[string]string `json:"params"`
	}
	require.NoError(t, json.Unmarshal(<-f.out, &req))
	require.Equal(t, "a\nb", req.Params["s"])
	// A raw newline between tokens is one frame here; a line reader would split it.
	f.in <- []byte(`{"id":` + string(req.ID) + ",\n" + `"result":"x\ny"}`)

	require.JSONEq(t, `"x\ny"`, string(<-answered))
}

func TestTransport_NotifySendsOneUnframedMessage(t *testing.T) {
	f := newFake(t)
	conn := overTransport(t, f)

	require.NoError(t, conn.Notify("progress", nil))

	msg := <-f.out
	require.NotEqual(t, byte('\n'), msg[len(msg)-1], "the transport owns framing")
	require.JSONEq(t, `{"jsonrpc":"2.0","method":"progress","params":null}`, string(msg))
}

func TestTransport_ServerRequestIsRepliedTo(t *testing.T) {
	f := newFake(t)
	conn := overTransport(t, f)
	f.in <- []byte(`{"id":7,"method":"ask","params":{}}`)

	frame := <-conn.Frames()
	require.Equal(t, "ask", frame.Method)
	require.NoError(t, conn.Reply(frame.ID, json.RawMessage(`{"ok":true}`)))

	require.JSONEq(t, `{"jsonrpc":"2.0","id":7,"result":{"ok":true}}`, string(<-f.out))
}

func TestTransport_RecvErrorEndsPendingCallsAndFrames(t *testing.T) {
	f := newFake(t)
	conn := overTransport(t, f)
	done := make(chan error, 1)
	go func() {
		_, err := conn.Call(t.Context(), "slow", nil)
		done <- err
	}()
	<-f.out

	close(f.in)

	require.ErrorContains(t, <-done, "connection closed")
	_, open := <-conn.Frames()
	require.False(t, open)
}

func TestTransport_SendFailureFailsTheCall(t *testing.T) {
	f := newFake(t)
	f.sendErr = errors.New("socket gone")
	conn := overTransport(t, f)

	_, err := conn.Call(t.Context(), "initialize", nil)

	require.ErrorContains(t, err, "socket gone")
}

func TestTransport_OverflowClosesTheTransport(t *testing.T) {
	f := newFake(t)
	conn := jsonrpc.NewTransport(f, jsonrpc.WithMaxQueued(1))
	t.Cleanup(func() { _ = conn.Close() })
	for range 3 {
		f.in <- []byte(`{"method":"n"}`)
	}

	<-f.done
	require.True(t, conn.Overflowed())
}
