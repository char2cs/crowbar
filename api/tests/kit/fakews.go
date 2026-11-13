package kit

import (
	"crypto/rand"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

// FakeWS is a WebSocket endpoint whose tickets work once. Text frames the client
// sends arrive on Frames; Accepted gets one value per completed handshake.
type FakeWS struct {
	Frames   chan []byte
	Accepted chan struct{}

	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	tickets map[string]bool // ticket -> already used
	conn    *websocket.Conn
	closing chan struct{}
}

// NewFakeWS starts the server and stops it on t.Cleanup.
func NewFakeWS(t *testing.T, protocols ...string) *FakeWS {
	t.Helper()
	f := &FakeWS{
		t:        t,
		Frames:   make(chan []byte, 16),
		Accepted: make(chan struct{}, 16),
		tickets:  map[string]bool{},
		closing:  make(chan struct{}),
	}
	up := websocket.Upgrader{Subprotocols: protocols}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !f.redeem(r) {
			http.Error(w, "ticket rejected", http.StatusUnauthorized)
			return
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		f.mu.Lock()
		f.conn = conn
		f.mu.Unlock()
		f.Accepted <- struct{}{}
		f.pump(conn)
	}))
	t.Cleanup(func() {
		close(f.closing)
		f.Drop()
		f.srv.Close()
	})
	return f
}

// NewTicket returns a fresh ticket the server accepts exactly once.
func (f *FakeWS) NewTicket() string {
	ticket := rand.Text()
	f.mu.Lock()
	f.tickets[ticket] = false
	f.mu.Unlock()
	return ticket
}

// URL is the endpoint carrying ticket as a query parameter.
func (f *FakeWS) URL(ticket string) string {
	return "ws" + strings.TrimPrefix(f.srv.URL, "http") + "/?ticket=" + ticket
}

// Send writes one text frame to the connected client.
func (f *FakeWS) Send(msg []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conn == nil {
		return websocket.ErrCloseSent
	}
	return f.conn.WriteMessage(websocket.TextMessage, msg)
}

// Drop cuts the connection without a close frame.
func (f *FakeWS) Drop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conn != nil {
		if err := f.conn.UnderlyingConn().Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			f.t.Logf("kit: drop websocket: %v", err)
		}
	}
}

func (f *FakeWS) redeem(r *http.Request) bool {
	ticket := r.URL.Query().Get("ticket")
	if ticket == "" {
		ticket = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	used, known := f.tickets[ticket]
	if !known || used {
		return false
	}
	f.tickets[ticket] = true
	return true
}

func (f *FakeWS) pump(conn *websocket.Conn) {
	for {
		kind, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if kind != websocket.TextMessage {
			continue
		}
		select {
		case f.Frames <- msg:
		case <-f.closing:
			return
		}
	}
}
