package ws_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	ws "github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

// BenchmarkBroadcaster_PushFanOut measures the broadcaster's hot path: a single
// serialization followed by fan-out to a fixed number of connected, filtered
// clients. Crowbar is an IDE where a file-write burst fans GitStatus and
// FileChangeEvent out to every open workspace pane, so Push latency is on the
// critical path (03 §5).
func BenchmarkBroadcaster_PushFanOut(b *testing.B) {
	const clients = 64

	gin.SetMode(gin.TestMode)
	br := ws.NewBroadcaster(itemDef())
	r := gin.New()
	r.GET("/items", br.Handle)
	srv := httptest.NewServer(r)
	b.Cleanup(srv.Close)

	conns := dialN(b, srv, clients)
	b.Cleanup(func() {
		for _, c := range conns {
			_ = c.Close()
		}
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		br.Push(item{Name: "alpha", Kind: "fruit"})
	}
}

// BenchmarkBroadcaster_PrefixNamespaceMatch measures the hierarchical
// segment-prefix match used to scope WS fan-out (03 §5). A repo-scoped client
// evaluates this against every event namespace on the Push hot path, so its
// per-call cost matters under a file-write burst.
func BenchmarkBroadcaster_PrefixNamespaceMatch(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ws.PrefixMatch("proj/repo", "proj/repo/workspace")
	}
}

func dialN(
	b *testing.B,
	srv *httptest.Server,
	n int,
) []*websocket.Conn {
	b.Helper()
	url := "ws" + srv.URL[len("http"):] + "/items?kind=fruit"
	conns := make([]*websocket.Conn, 0, n)
	for i := 0; i < n; i++ {
		conn := dialBench(b, url)
		conns = append(conns, conn)
	}
	return conns
}

func dialBench(
	b *testing.B,
	url string,
) *websocket.Conn {
	b.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil) //nolint:bodyclose
	if err != nil {
		b.Fatalf("dial: %v", err)
	}
	return conn
}
