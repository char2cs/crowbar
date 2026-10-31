package ws_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

var short = ws.KeepAlive{Ping: 10 * time.Millisecond, Pong: 200 * time.Millisecond, Write: time.Second}

const bound = 10 * time.Second

// keptAlive serves one socket under short and reports when its read ends.
func keptAlive(t *testing.T) (*websocket.Conn, <-chan struct{}) {
	t.Helper()
	readEnded := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		done := make(chan struct{})
		defer close(done)
		if short.Arm(conn) != nil {
			return
		}
		short.Pings(conn, done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				close(readEnded)
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() { _ = conn.Close() })
	return conn, readEnded
}

func TestKeepAlive_APeerThatPongsStaysAndIsPinged(t *testing.T) {
	conn, readEnded := keptAlive(t)
	pings := make(chan struct{}, 64)
	conn.SetPingHandler(func(data string) error {
		pings <- struct{}{}
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(time.Second))
	})
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// Sixty pings span three pong timeouts: answering them is what keeps the socket.
	for range 60 {
		select {
		case <-pings:
		case <-readEnded:
			require.FailNow(t, "a peer that pongs was cut")
		case <-time.After(bound):
			require.FailNow(t, "no ping arrived")
		}
	}
}

func TestKeepAlive_APeerThatNeverPongsIsCut(t *testing.T) {
	conn, readEnded := keptAlive(t)
	conn.SetPingHandler(func(string) error { return nil })
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	select {
	case <-readEnded:
	case <-time.After(bound):
		require.FailNow(t, "a silent peer was never cut")
	}
}
