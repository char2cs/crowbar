package wshub_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/char2cs/crowbar/api/internal/wshub"
	"github.com/gorilla/websocket"
)

func TestHub_BroadcastReachesClient(t *testing.T) {
	h := wshub.New()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		h.Register(conn)
		defer h.Unregister(conn)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	url := "ws" + srv.URL[4:]
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	type payload struct{ OK bool }
	h.Broadcast(payload{OK: true})

	_, msg, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(msg) != `{"OK":true}` {
		t.Fatalf("unexpected message: %s", msg)
	}
}
