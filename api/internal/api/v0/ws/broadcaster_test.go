package ws_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ws "github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

type item struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

func itemDef() ws.StreamDef[item] {
	return ws.StreamDef[item]{
		Namespace: func(i item) string { return i.Name },
		Serialize: func(i item) ([]byte, error) { return json.Marshal(i) },
		Filters: []ws.FilterDef[item]{
			{Param: "kind", Extract: func(i item) string { return i.Kind }, Match: ws.ExactMatch},
		},
	}
}

func setup(
	t *testing.T,
	def ws.StreamDef[item],
) (*ws.Broadcaster[item], *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	b := ws.NewBroadcaster(def)
	r := gin.New()
	r.GET("/items", b.Handle)
	r.GET("/items/:ns", b.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return b, srv
}

func dial(
	t *testing.T,
	srv *httptest.Server,
	path string,
) *websocket.Conn {
	t.Helper()
	url := "ws" + srv.URL[len("http"):] + path
	conn, _, err := websocket.DefaultDialer.Dial(url, nil) //nolint:bodyclose
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func read(
	t *testing.T,
	conn *websocket.Conn,
	v any,
) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(msg, v))
}

func TestBroadcaster_Push_DeliversToClient(t *testing.T) {
	b, srv := setup(t, itemDef())
	conn := dial(t, srv, "/items")
	b.WaitRegistered()

	b.Push(item{Name: "alpha", Kind: "fruit"})

	var got item
	read(t, conn, &got)
	assert.Equal(t, "alpha", got.Name)
}

func TestBroadcaster_Push_FieldFilter(t *testing.T) {
	b, srv := setup(t, itemDef())
	conn := dial(t, srv, "/items?kind=fruit")
	b.WaitRegistered()

	b.Push(item{Name: "carrot", Kind: "veg"})
	b.Push(item{Name: "apple", Kind: "fruit"})

	var got item
	read(t, conn, &got)
	assert.Equal(t, "apple", got.Name)
}

func TestBroadcaster_NamespaceGlob(t *testing.T) {
	b, srv := setup(t, itemDef())
	conn := dial(t, srv, "/items/alpha")
	b.WaitRegistered()

	b.Push(item{Name: "beta", Kind: "fruit"})
	b.Push(item{Name: "alpha", Kind: "fruit"})

	var got item
	read(t, conn, &got)
	assert.Equal(t, "alpha", got.Name)
}

func TestBroadcaster_SnapshotOnSubscribe(t *testing.T) {
	def := itemDef()
	def.Snapshot = func() []item {
		return []item{{Name: "seed", Kind: "fruit"}}
	}
	b, srv := setup(t, def)
	conn := dial(t, srv, "/items")
	b.WaitRegistered()

	var got item
	read(t, conn, &got)
	assert.Equal(t, "seed", got.Name)
}

// readItem reads one frame with a deadline and decodes it, returning false on
// any read error so callers can loop until an expected count is reached without
// a fixed sleep.
func readItem(
	t *testing.T,
	conn *websocket.Conn,
) (item, bool) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return item{}, false
	}
	var got item
	require.NoError(t, json.Unmarshal(msg, &got))
	return got, true
}

// A snapshot larger than the cl.send buffer (64) must be delivered in full: the
// non-dropping snapshot path must not truncate past the buffer size.
func TestBroadcaster_Snapshot_LargerThanBuffer_NoTruncation(t *testing.T) {
	const total = 200
	def := itemDef()
	def.Snapshot = func() []item {
		out := make([]item, total)
		for i := range out {
			out[i] = item{Name: fmt.Sprintf("s%d", i), Kind: "fruit"}
		}
		return out
	}
	b, srv := setup(t, def)
	conn := dial(t, srv, "/items?kind=fruit")
	b.WaitRegistered()

	seen := make(map[string]struct{}, total)
	for len(seen) < total {
		got, ok := readItem(t, conn)
		require.Truef(t, ok, "stopped after %d/%d snapshot frames (truncation)", len(seen), total)
		seen[got.Name] = struct{}{}
	}
	assert.Len(t, seen, total)
}

// Frames that do not match the client's predicate are excluded from the snapshot
// (the snapshot is filtered, like live Push).
func TestBroadcaster_Snapshot_FilteredByPredicate(t *testing.T) {
	def := itemDef()
	def.Snapshot = func() []item {
		return []item{
			{Name: "veg1", Kind: "veg"},
			{Name: "fruit1", Kind: "fruit"},
		}
	}
	b, srv := setup(t, def)
	conn := dial(t, srv, "/items?kind=fruit")
	b.WaitRegistered()

	got, ok := readItem(t, conn)
	require.True(t, ok)
	assert.Equal(t, "fruit1", got.Name, "only matching snapshot frames should arrive")
}

// A snapshot item that fails to serialize is skipped; the remaining items are
// still delivered in full (serialize error must not abort the snapshot).
func TestBroadcaster_Snapshot_SerializeErrorSkipped(t *testing.T) {
	def := ws.StreamDef[item]{
		Namespace: func(i item) string { return i.Name },
		Serialize: func(i item) ([]byte, error) {
			if i.Name == "bad" {
				return nil, fmt.Errorf("boom")
			}
			return json.Marshal(i)
		},
		Filters: []ws.FilterDef[item]{
			{Param: "kind", Extract: func(i item) string { return i.Kind }, Match: ws.ExactMatch},
		},
		Snapshot: func() []item {
			return []item{
				{Name: "bad", Kind: "fruit"},
				{Name: "good", Kind: "fruit"},
			}
		},
	}
	b, srv := setup(t, def)
	conn := dial(t, srv, "/items?kind=fruit")
	b.WaitRegistered()

	got, ok := readItem(t, conn)
	require.True(t, ok)
	assert.Equal(t, "good", got.Name, "non-failing snapshot frames must still arrive")
}

// The snapshot frames must precede live frames on the wire even when the snapshot
// is large (ordering preserved by flushSnapshot-before-loop).
func TestBroadcaster_Snapshot_PrecedesLive(t *testing.T) {
	const total = 100
	def := itemDef()
	def.Snapshot = func() []item {
		out := make([]item, total)
		for i := range out {
			out[i] = item{Name: fmt.Sprintf("snap%d", i), Kind: "fruit"}
		}
		return out
	}
	b, srv := setup(t, def)
	conn := dial(t, srv, "/items?kind=fruit")
	b.WaitRegistered()

	b.Push(item{Name: "live", Kind: "fruit"})

	for i := 0; i < total; i++ {
		got, ok := readItem(t, conn)
		require.Truef(t, ok, "missing snapshot frame %d before live", i)
		require.NotEqual(t, "live", got.Name, "live frame arrived before snapshot completed")
	}
	got, ok := readItem(t, conn)
	require.True(t, ok)
	assert.Equal(t, "live", got.Name, "live frame must follow all snapshot frames")
}

// A live Push to a DIFFERENT client must not be blocked while a client with a
// slow/large snapshot connects: the broadcaster lock is not held during snapshot
// compute. Timing-free: the snapshot func signals it is running and blocks until
// released; meanwhile a Push to an already-connected client must complete.
func TestBroadcaster_Snapshot_DoesNotBlockConcurrentPush(t *testing.T) {
	running := make(chan struct{})
	release := make(chan struct{})
	var armed atomic.Bool

	def := itemDef()
	def.Snapshot = func() []item {
		if !armed.Load() {
			return nil
		}
		close(running)
		<-release
		return []item{{Name: "snap", Kind: "fruit"}}
	}
	b, srv := setup(t, def)

	// First client connects with snapshot disarmed (empty snapshot).
	first := dial(t, srv, "/items?kind=fruit")
	b.WaitRegistered()

	// Arm the snapshot, then connect a second client whose snapshot blocks.
	armed.Store(true)
	go func() { _ = dial(t, srv, "/items?kind=fruit") }()
	<-running // second client is inside Snapshot(), NOT holding b.mu

	// Push to the first client must succeed while the second is mid-snapshot.
	pushed := make(chan struct{})
	go func() {
		b.Push(item{Name: "concurrent", Kind: "fruit"})
		close(pushed)
	}()

	select {
	case <-pushed:
	case <-time.After(2 * time.Second):
		t.Fatal("Push blocked while another client computed its snapshot under lock")
	}

	got, ok := readItem(t, first)
	require.True(t, ok)
	assert.Equal(t, "concurrent", got.Name)

	close(release)
}

func TestBroadcaster_UpgradeRejectsNonWS(t *testing.T) {
	_, srv := setup(t, itemDef())
	resp, err := http.Get(srv.URL + "/items") //nolint:noctx
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBroadcaster_SlowConsumer_DoesNotBlock(t *testing.T) {
	b, srv := setup(t, itemDef())
	_ = dial(t, srv, "/items")
	b.WaitRegistered()
	for i := 0; i < 65; i++ {
		b.Push(item{Name: fmt.Sprintf("i%d", i), Kind: "fruit"})
	}
}

func TestBroadcaster_SerializationError_SkipsDelivery(t *testing.T) {
	def := ws.StreamDef[item]{
		Namespace: func(i item) string { return i.Name },
		Serialize: func(i item) ([]byte, error) { return nil, fmt.Errorf("boom") },
	}
	b, srv := setup(t, def)
	conn := dial(t, srv, "/items")
	b.WaitRegistered()
	b.Push(item{Name: "x"})
	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	assert.Error(t, err)
}
