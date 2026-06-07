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
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	_ = server.Close()

	require.False(t, writeNext(server, cl, ticker))
}

func TestWriteNext_DoneReturnsFalse(t *testing.T) {
	server, _, cleanup := upgradedPair(t)
	defer cleanup()

	cl := newClient()
	close(cl.done)
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	require.False(t, writeNext(server, cl, ticker))
}

func TestWriteNext_PingWriteErrorReturnsFalse(t *testing.T) {
	server, _, cleanup := upgradedPair(t)
	defer cleanup()

	cl := newClient()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	_ = server.Close()
	<-ticker.C

	require.False(t, writeNext(server, cl, ticker))
}

func TestWriteNext_SendSuccess(t *testing.T) {
	server, client, cleanup := upgradedPair(t)
	defer cleanup()

	cl := newClient()
	cl.send <- []byte("frame")
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	require.True(t, writeNext(server, cl, ticker))

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

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("readPump did not return after conn closed")
	}
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
	// closure executes before the loop returns.
	err := client.WriteControl(
		websocket.PongMessage,
		nil,
		time.Now().Add(time.Second),
	)
	require.NoError(t, err)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte("x")))
	_ = client.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("readPump did not return")
	}
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

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("writePump did not return after snapshot flush failed")
	}
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

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("writePump did not return after live write failed")
	}
}
