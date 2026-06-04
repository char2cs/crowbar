package ws_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
