package wsconn_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/connect"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/wsconn"
	"github.com/char2cs/crowbar/api/tests/kit"
)

const wait = 5 * time.Second

func freshTickets(f *kit.FakeWS) func(context.Context) (connect.Endpoint, error) {
	return func(context.Context) (connect.Endpoint, error) {
		return connect.Endpoint{URL: f.URL(f.NewTicket())}, nil
	}
}

func recvFrame(t *testing.T, f *kit.FakeWS) []byte {
	t.Helper()
	select {
	case b := <-f.Frames:
		return b
	case <-time.After(wait):
		t.Fatal("no frame received")
		return nil
	}
}

func TestRoundTripThroughJSONRPC(t *testing.T) {
	f := kit.NewFakeWS(t, "proto")
	tr, err := wsconn.Dial(context.Background(), freshTickets(f))
	require.NoError(t, err)
	conn := jsonrpc.NewTransport(tr)
	t.Cleanup(func() { _ = conn.Close() })

	type result struct {
		raw json.RawMessage
		err error
	}
	done := make(chan result, 1)
	go func() {
		raw, err := conn.Call(context.Background(), "ping", map[string]int{"n": 1})
		done <- result{raw, err}
	}()
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	require.NoError(t, json.Unmarshal(recvFrame(t, f), &req))
	require.Equal(t, "ping", req.Method)
	require.NoError(t, f.Send([]byte(`{"jsonrpc":"2.0","id":`+string(req.ID)+`,"result":{"ok":true}}`)))
	select {
	case r := <-done:
		require.NoError(t, r.err)
		require.JSONEq(t, `{"ok":true}`, string(r.raw))
	case <-time.After(wait):
		t.Fatal("call never completed")
	}

	require.NoError(t, f.Send([]byte(`{"jsonrpc":"2.0","id":7,"method":"ask","params":{}}`)))
	select {
	case fr := <-conn.Frames():
		require.Equal(t, "ask", fr.Method)
		require.NoError(t, conn.Reply(fr.ID, json.RawMessage(`"yes"`)))
	case <-time.After(wait):
		t.Fatal("server request never delivered")
	}
	require.JSONEq(t, `{"jsonrpc":"2.0","id":7,"result":"yes"}`, string(recvFrame(t, f)))
}

func TestFrameWithEmbeddedNewlineIsOneMessage(t *testing.T) {
	f := kit.NewFakeWS(t)
	tr, err := wsconn.Dial(context.Background(), freshTickets(f))
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.Close() })

	msg := []byte("{\"a\":\n1}")
	require.NoError(t, tr.Send(msg))
	require.Equal(t, msg, recvFrame(t, f))

	require.NoError(t, f.Send(msg))
	got, err := tr.Recv()
	require.NoError(t, err)
	require.Equal(t, msg, got)
}

func TestDialReResolvesOnceAfterRejectedTicket(t *testing.T) {
	f := kit.NewFakeWS(t)
	stale := f.NewTicket()
	first, err := wsconn.Dial(context.Background(), func(context.Context) (connect.Endpoint, error) {
		return connect.Endpoint{URL: f.URL(stale)}, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })

	calls := 0
	tr, err := wsconn.Dial(context.Background(), func(context.Context) (connect.Endpoint, error) {
		calls++
		if calls == 1 {
			return connect.Endpoint{URL: f.URL(stale)}, nil
		}
		return connect.Endpoint{URL: f.URL(f.NewTicket())}, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.Close() })
	require.Equal(t, 2, calls)

	calls = 0
	_, err = wsconn.Dial(context.Background(), func(context.Context) (connect.Endpoint, error) {
		calls++
		return connect.Endpoint{URL: f.URL(stale)}, nil
	})
	require.Error(t, err)
	require.Equal(t, 2, calls)
}

func TestDialSurfacesResolveError(t *testing.T) {
	boom := errors.New("boom")
	_, err := wsconn.Dial(context.Background(), func(context.Context) (connect.Endpoint, error) {
		return connect.Endpoint{}, boom
	})
	require.ErrorIs(t, err, boom)
}

func TestDropFailsRecv(t *testing.T) {
	f := kit.NewFakeWS(t)
	tr, err := wsconn.Dial(context.Background(), freshTickets(f))
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.Close() })
	<-f.Accepted

	errc := make(chan error, 1)
	go func() { _, err := tr.Recv(); errc <- err }()
	f.Drop()
	select {
	case err := <-errc:
		require.Error(t, err)
	case <-time.After(wait):
		t.Fatal("Recv did not fail after drop")
	}
}

func TestCloseIsIdempotentAndEndsRecv(t *testing.T) {
	f := kit.NewFakeWS(t)
	tr, err := wsconn.Dial(context.Background(), freshTickets(f))
	require.NoError(t, err)

	errc := make(chan error, 1)
	go func() { _, err := tr.Recv(); errc <- err }()
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); _ = tr.Close() }()
	}
	wg.Wait()
	require.NoError(t, tr.Close())
	select {
	case err := <-errc:
		require.Error(t, err)
	case <-time.After(wait):
		t.Fatal("Recv did not return after Close")
	}
	require.Error(t, tr.Send([]byte("{}")))
	_, err = tr.Recv()
	require.Error(t, err)
}
