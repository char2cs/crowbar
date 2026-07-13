package ws_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

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

// readItem blocks until the next frame arrives, then decodes it.
//
// There is deliberately NO read deadline. The arrival of the frame IS the
// signal, so a blocking read is the honest instrument: it returns the instant
// the broadcaster delivers, however loaded the machine is. A frame that never
// arrives hangs until `go test -timeout` fires, which fails the run with a full
// goroutine dump pointing at the stuck reader — strictly better diagnostics than
// a 2-second guess that turns a slow machine into a red build.
func readItem(
	t *testing.T,
	conn *websocket.Conn,
) item {
	t.Helper()
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got item
	require.NoError(t, json.Unmarshal(msg, &got))
	return got
}

func TestBroadcaster_Push_DeliversToClient(t *testing.T) {
	b, srv := setup(t, itemDef())
	conn := dial(t, srv, "/items")
	b.WaitRegistered()

	b.Push(item{Name: "alpha", Kind: "fruit"})

	assert.Equal(t, "alpha", readItem(t, conn).Name)
}

func TestBroadcaster_Push_FieldFilter(t *testing.T) {
	b, srv := setup(t, itemDef())
	conn := dial(t, srv, "/items?kind=fruit")
	b.WaitRegistered()

	b.Push(item{Name: "carrot", Kind: "veg"})
	b.Push(item{Name: "apple", Kind: "fruit"})

	assert.Equal(t, "apple", readItem(t, conn).Name)
}

func TestBroadcaster_NamespaceGlob(t *testing.T) {
	b, srv := setup(t, itemDef())
	conn := dial(t, srv, "/items/alpha")
	b.WaitRegistered()

	b.Push(item{Name: "beta", Kind: "fruit"})
	b.Push(item{Name: "alpha", Kind: "fruit"})

	assert.Equal(t, "alpha", readItem(t, conn).Name)
}

func TestBroadcaster_SnapshotOnSubscribe(t *testing.T) {
	def := itemDef()
	def.Snapshot = func(_ string) []item {
		return []item{{Name: "seed", Kind: "fruit"}}
	}
	b, srv := setup(t, def)
	conn := dial(t, srv, "/items")
	b.WaitRegistered()

	assert.Equal(t, "seed", readItem(t, conn).Name)
}

// TestBroadcaster_ScopedSnapshot asserts the connecting client's hierarchical
// scope ("p/r/w" from projectId/repoId/wsId path params, empties trimmed) is
// passed through to def.Snapshot, so per-entity lazy storage reads only the
// subscribed subtree.
func TestBroadcaster_ScopedSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gotScope := make(chan string, 1)
	def := itemDef()
	def.Snapshot = func(scope string) []item {
		gotScope <- scope
		// Namespace must fall under the subscribed scope so the live prefix
		// predicate admits it (PrefixMatch("p1/r1/w1", "p1/r1/w1")).
		return []item{{Name: "p1/r1/w1", Kind: "fruit"}}
	}
	b := ws.NewBroadcaster(def)
	r := gin.New()
	r.GET("/p/:projectId/r/:repoId/w/:wsId/items", b.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dial(t, srv, "/p/p1/r/r1/w/w1/items")
	b.WaitNRegistered(1)

	assert.Equal(t, "p1/r1/w1", readItem(t, conn).Name)
	assert.Equal(t, "p1/r1/w1", <-gotScope)
}

// TestBroadcaster_ScopedSnapshot_RepoLevelTrimsEmpty asserts a repo-level
// subscription (no wsId) trims the trailing empty segment, yielding "p/r".
func TestBroadcaster_ScopedSnapshot_RepoLevelTrimsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gotScope := make(chan string, 1)
	def := itemDef()
	def.Snapshot = func(scope string) []item {
		gotScope <- scope
		return nil
	}
	b := ws.NewBroadcaster(def)
	r := gin.New()
	r.GET("/p/:projectId/r/:repoId/items", b.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	_ = dial(t, srv, "/p/p1/r/r1/items")
	b.WaitNRegistered(1)

	assert.Equal(t, "p1/r1", <-gotScope)
}

// TestBroadcaster_PrefixNamespace_RepoScopedReceivesChildren asserts a
// repo-level subscription ("p1/r1", derived from projectId/repoId path params)
// receives every child workspace's live events via hierarchical PrefixMatch
// ("p1/r1" matches "p1/r1/w1"), while events under a sibling repo ("p1/r2/w1")
// are filtered out.
func TestBroadcaster_PrefixNamespace_RepoScopedReceivesChildren(t *testing.T) {
	gin.SetMode(gin.TestMode)
	b := ws.NewBroadcaster(itemDef())
	r := gin.New()
	r.GET("/p/:projectId/r/:repoId/items", b.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dial(t, srv, "/p/p1/r/r1/items")
	b.WaitNRegistered(1)

	// Sibling-repo event must be filtered out; both child-workspace events under
	// the subscribed repo must arrive, in order. The filtered event is proven
	// dropped by ordering, not by a clock: the client's send channel and its
	// writePump are strictly FIFO, so a wrongly-admitted "p1/r2/w1" could only
	// arrive BEFORE the first child.
	b.Push(item{Name: "p1/r2/w1", Kind: "fruit"})
	b.Push(item{Name: "p1/r1/w1", Kind: "fruit"})
	b.Push(item{Name: "p1/r1/w2", Kind: "fruit"})

	assert.Equal(t, "p1/r1/w1", readItem(t, conn).Name,
		"sibling-repo event must be filtered, first child must arrive")
	assert.Equal(t, "p1/r1/w2", readItem(t, conn).Name,
		"second child-workspace event must arrive")
}

// A snapshot larger than the cl.send buffer (64) must be delivered in full: the
// non-dropping snapshot path must not truncate past the buffer size.
func TestBroadcaster_Snapshot_LargerThanBuffer_NoTruncation(t *testing.T) {
	const total = 200
	def := itemDef()
	def.Snapshot = func(_ string) []item {
		out := make([]item, total)
		for i := range out {
			out[i] = item{Name: fmt.Sprintf("s%d", i), Kind: "fruit"}
		}
		return out
	}
	b, srv := setup(t, def)
	conn := dial(t, srv, "/items?kind=fruit")
	b.WaitRegistered()

	// Blocking reads: a truncated snapshot stops delivering and the read hangs
	// until `go test -timeout` fires, naming this test. No deadline is needed to
	// turn truncation into a failure.
	seen := make(map[string]struct{}, total)
	for len(seen) < total {
		seen[readItem(t, conn).Name] = struct{}{}
	}
	assert.Len(t, seen, total)
}

// Frames that do not match the client's predicate are excluded from the snapshot
// (the snapshot is filtered, like live Push).
func TestBroadcaster_Snapshot_FilteredByPredicate(t *testing.T) {
	def := itemDef()
	def.Snapshot = func(_ string) []item {
		return []item{
			{Name: "veg1", Kind: "veg"},
			{Name: "fruit1", Kind: "fruit"},
		}
	}
	b, srv := setup(t, def)
	conn := dial(t, srv, "/items?kind=fruit")
	b.WaitRegistered()

	assert.Equal(t, "fruit1", readItem(t, conn).Name,
		"only matching snapshot frames should arrive")
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
		Snapshot: func(_ string) []item {
			return []item{
				{Name: "bad", Kind: "fruit"},
				{Name: "good", Kind: "fruit"},
			}
		},
	}
	b, srv := setup(t, def)
	conn := dial(t, srv, "/items?kind=fruit")
	b.WaitRegistered()

	assert.Equal(t, "good", readItem(t, conn).Name,
		"non-failing snapshot frames must still arrive")
}

// The snapshot frames must precede live frames on the wire even when the snapshot
// is large (ordering preserved by flushSnapshot-before-loop).
func TestBroadcaster_Snapshot_PrecedesLive(t *testing.T) {
	const total = 100
	def := itemDef()
	def.Snapshot = func(_ string) []item {
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
		require.NotEqualf(t, "live", readItem(t, conn).Name,
			"live frame arrived before snapshot frame %d", i)
	}
	assert.Equal(t, "live", readItem(t, conn).Name,
		"live frame must follow all snapshot frames")
}

// A live Push to a DIFFERENT client must not be blocked while a client with a
// slow/large snapshot connects: the broadcaster lock is not held during snapshot
// compute.
//
// The instrument is a handshake, not a clock. The snapshot func parks inside
// Snapshot() (holding no broadcaster lock) until `release` is closed, and
// `release` is closed only by the deferred cleanup — i.e. strictly AFTER the
// assertion below. So a Push that genuinely needed b.mu could never reach
// close(pushed), and `<-pushed` would block forever. That is the real signal:
// the test passes the instant the concurrent Push completes, and a regression
// deadlocks into `go test -timeout`, which dumps both goroutines showing Push
// parked on b.mu.RLock while snapshotFor holds it — a far better diagnosis than
// any duration a 30-second guess could have produced.
func TestBroadcaster_Snapshot_DoesNotBlockConcurrentPush(t *testing.T) {
	running := make(chan struct{})
	release := make(chan struct{})
	// Deferred, so the parked snapshot goroutine is always freed — including when
	// an assertion below fails and the test returns early.
	defer close(release)
	var armed atomic.Bool

	def := itemDef()
	def.Snapshot = func(_ string) []item {
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

	// Push to the first client must complete while the second is mid-snapshot.
	pushed := make(chan struct{})
	go func() {
		b.Push(item{Name: "concurrent", Kind: "fruit"})
		close(pushed)
	}()

	<-pushed // the real signal: Push returned without needing the snapshot's lock

	assert.Equal(t, "concurrent", readItem(t, first).Name)
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

// An event whose Serialize fails is never delivered.
//
// "Nothing arrives" cannot be established by waiting — a 100ms read deadline
// only proves nothing arrived within 100ms. It is established here by ORDERING:
// a following event that DOES serialize must be the very FIRST frame the client
// receives. The client's send channel and its writePump are strictly FIFO, so a
// wrongly-delivered unserializable frame could only ever arrive BEFORE the
// sentinel. Reading the sentinel first is therefore a positive, deterministic
// proof that the bad frame was skipped.
func TestBroadcaster_SerializationError_SkipsDelivery(t *testing.T) {
	def := ws.StreamDef[item]{
		Namespace: func(i item) string { return i.Name },
		Serialize: func(i item) ([]byte, error) {
			if i.Kind == "unserializable" {
				return nil, fmt.Errorf("boom")
			}
			return json.Marshal(i)
		},
	}
	b, srv := setup(t, def)
	conn := dial(t, srv, "/items")
	b.WaitRegistered()

	b.Push(item{Name: "dropped", Kind: "unserializable"})
	b.Push(item{Name: "sentinel", Kind: "ok"})

	assert.Equal(t, "sentinel", readItem(t, conn).Name,
		"the unserializable event must be skipped, not delivered")
}
