package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func upgradedPair(
	t *testing.T,
) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()
	serverConnCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		c, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		serverConnCh <- c
	}))
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, resp, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	if resp != nil {
		_ = resp.Body.Close()
	}
	server := <-serverConnCh
	cleanup := func() {
		_ = client.Close()
		_ = server.Close()
		srv.Close()
	}
	return server, client, cleanup
}

// inertTicker is a ticker that can never fire: nothing ever sends on its
// channel. It pins writeNext's ping arm shut so the select is forced onto the
// arm under test, without a real clock running in the background. (A
// time.NewTicker(time.Hour) would also not fire, but a channel with no sender
// states the intent — "this arm must not be taken" — as a fact rather than a
// bet on the test finishing within the hour.)
//
// Stop() is deliberately never called on these: a hand-built Ticker has no
// runtime timer behind it, and it needs no cleanup.
func inertTicker() *time.Ticker {
	return &time.Ticker{C: make(chan time.Time)}
}

// firedTicker is a ticker whose tick has ALREADY been delivered into its
// buffer. writeNext's select is therefore guaranteed to take the ping arm (the
// send channel is empty and done is open, so no other arm is ready) — the ping
// path becomes deterministic instead of racing a real 1ms ticker.
func firedTicker() *time.Ticker {
	c := make(chan time.Time, 1)
	c <- time.Time{}
	return &time.Ticker{C: c}
}

func TestFlushSnapshot_WriteErrorAborts(t *testing.T) {
	server, _, cleanup := upgradedPair(t)
	defer cleanup()

	cl := newClient()
	_ = server.Close() // closed conn: SetWriteDeadline + WriteMessage both error

	ok := flushSnapshot(server, cl, [][]byte{[]byte("frame")})
	require.False(t, ok)
}

func TestFlushSnapshot_DoneMidFlushAborts(t *testing.T) {
	server, _, cleanup := upgradedPair(t)
	defer cleanup()

	cl := newClient()
	close(cl.done) // torn down before the first frame

	ok := flushSnapshot(server, cl, [][]byte{[]byte("frame")})
	require.False(t, ok)
}

func TestFlushSnapshot_EmptySnapshotSucceeds(t *testing.T) {
	server, _, cleanup := upgradedPair(t)
	defer cleanup()

	cl := newClient()
	require.True(t, flushSnapshot(server, cl, nil))
}

func TestWriteNext_SendWriteErrorReturnsFalse(t *testing.T) {
	server, _, cleanup := upgradedPair(t)
	defer cleanup()

	cl := newClient()
	cl.send <- []byte("frame")
	_ = server.Close()

	require.False(t, writeNext(server, cl, inertTicker()))
}

func TestWriteNext_DoneReturnsFalse(t *testing.T) {
	server, _, cleanup := upgradedPair(t)
	defer cleanup()

	cl := newClient()
	close(cl.done)

	require.False(t, writeNext(server, cl, inertTicker()))
}

func TestWriteNext_PingWriteErrorReturnsFalse(t *testing.T) {
	server, _, cleanup := upgradedPair(t)
	defer cleanup()

	cl := newClient()
	_ = server.Close()

	// The tick is already buffered, so the ping arm is the only ready arm: the
	// select cannot go anywhere else, with no dependence on a real clock.
	require.False(t, writeNext(server, cl, firedTicker()))
}

func TestWriteNext_SendSuccess(t *testing.T) {
	server, client, cleanup := upgradedPair(t)
	defer cleanup()

	cl := newClient()
	cl.send <- []byte("frame")

	require.True(t, writeNext(server, cl, inertTicker()))

	_, msg, err := client.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "frame", string(msg))
}

func TestReadPump_ReturnsOnReadError(t *testing.T) {
	server, client, cleanup := upgradedPair(t)
	defer cleanup()

	done := make(chan struct{})
	go func() {
		readPump(server)
		close(done)
	}()

	_ = client.Close() // peer gone: NextReader errors and readPump returns

	// Blocking receive: the close of done IS the "readPump returned" signal. A
	// pump that wedges hangs here until `go test -timeout` fires and dumps the
	// stuck goroutine — a sharper failure than any 5-second guess, which would
	// only have restated the test timeout less reliably.
	<-done
}

func TestReadPump_PongHandlerResetsDeadline(t *testing.T) {
	server, client, cleanup := upgradedPair(t)
	defer cleanup()

	done := make(chan struct{})
	go func() {
		readPump(server) // installs its own pong handler
		close(done)
	}()

	// On the single ordered TCP stream the pong is delivered (and its handler
	// run) before the subsequent close is observed, so readPump's pong-handler
	// closure executes before the loop returns. The zero deadline means "no write
	// timeout": the control frame's delivery is ordered by the stream, not timed.
	require.NoError(t, client.WriteControl(websocket.PongMessage, nil, time.Time{}))
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte("x")))
	_ = client.Close()

	<-done
}

func TestWritePump_AbortsWhenSnapshotFlushFails(t *testing.T) {
	server, _, cleanup := upgradedPair(t)
	defer cleanup()

	cl := newClient()
	_ = server.Close()

	done := make(chan struct{})
	go func() {
		writePump(server, cl, [][]byte{[]byte("frame")})
		close(done)
	}()

	<-done
}

func TestWritePump_AbortsWhenWriteNextFails(t *testing.T) {
	server, _, cleanup := upgradedPair(t)
	defer cleanup()

	cl := newClient()
	cl.send <- []byte("frame")
	_ = server.Close() // closed conn: the live write errors immediately

	done := make(chan struct{})
	go func() {
		writePump(server, cl, nil)
		close(done)
	}()

	<-done
}
