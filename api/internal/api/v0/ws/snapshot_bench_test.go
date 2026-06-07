package ws_test

import (
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	ws "github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

// BenchmarkBroadcaster_SnapshotOnSubscribe measures the snapshot-on-subscribe hot
// path: when a client connects, the broadcaster serializes the full snapshot and
// streams it before any live event. Crowbar's sidebar issues this on every
// workspace/git/chats reconnect, so the per-connect serialization of a sizable
// snapshot is on the first-paint critical path (03 §1a).
func BenchmarkBroadcaster_SnapshotOnSubscribe(b *testing.B) {
	// Sized to the client send buffer so the whole snapshot is deliverable
	// without the broadcaster's drop-on-full backpressure firing mid-burst.
	const snapshotSize = 64

	gin.SetMode(gin.TestMode)
	br := ws.NewBroadcaster(snapshotDef(snapshotSize))
	r := gin.New()
	r.GET("/items", br.Handle)
	srv := httptest.NewServer(r)
	b.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/items?kind=fruit"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		drainSnapshot(b, url, snapshotSize)
	}
}

func snapshotDef(
	size int,
) ws.StreamDef[item] {
	rows := make([]item, 0, size)
	for i := 0; i < size; i++ {
		rows = append(rows, item{Name: "ws" + strconv.Itoa(i), Kind: "fruit"})
	}
	def := itemDef()
	def.Snapshot = func() []item { return rows }
	return def
}

func drainSnapshot(
	b *testing.B,
	url string,
	want int,
) {
	b.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil) //nolint:bodyclose
	if err != nil {
		b.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Read frames until the snapshot drains to a short idle gap. Drop-on-full
	// backpressure (03 §5) may shed a few frames under load, so the bench measures
	// the serialize+fan-out work delivered rather than asserting an exact count.
	read := 0
	for read < want {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		if _, _, readErr := conn.ReadMessage(); readErr != nil {
			break
		}
		read++
	}
	if read == 0 {
		b.Fatal("no snapshot frame delivered")
	}
}
