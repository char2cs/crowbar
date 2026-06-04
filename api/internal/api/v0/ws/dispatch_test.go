package ws_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ws "github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

func TestDispatch_RESTReturnsSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	b := ws.NewBroadcaster(ws.StreamDef[item]{
		Namespace: func(i item) string { return i.Name },
		Serialize: func(i item) ([]byte, error) { return json.Marshal(i) },
	})
	r := gin.New()
	r.GET("/items", ws.Dispatch(b, func(*gin.Context) (any, error) {
		return []item{{Name: "snap"}}, nil
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/items", nil)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "snap")
}

func TestDispatch_SnapshotError_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	b := ws.NewBroadcaster(ws.StreamDef[item]{Namespace: func(i item) string { return i.Name }})
	r := gin.New()
	r.GET("/items", ws.Dispatch(b, func(*gin.Context) (any, error) {
		return nil, assertErr{}
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/items", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestDispatch_WSUpgrade_RoutesToHandle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	b := ws.NewBroadcaster(ws.StreamDef[item]{
		Namespace: func(i item) string { return i.Name },
		Serialize: func(i item) ([]byte, error) { return json.Marshal(i) },
	})
	r := gin.New()
	r.GET("/items", ws.Dispatch(b, func(*gin.Context) (any, error) {
		return []item{{Name: "snap"}}, nil
	}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/items"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil) //nolint:bodyclose
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	b.WaitRegistered()

	b.Push(item{Name: "live"})
	var got item
	read(t, conn, &got)
	assert.Equal(t, "live", got.Name)
}

type assertErr struct{}

func (assertErr) Error() string { return "snapshot failed" }
