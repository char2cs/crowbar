package ws_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

func TestHoldRefusesAPlainGETWithoutAcquiring(t *testing.T) {
	gin.SetMode(gin.TestMode)
	acquired := false
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/attach", http.NoBody)

	ws.Hold(c, func() func() {
		acquired = true
		return func() {}
	})

	assert.False(t, acquired)
	assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
}

func TestHoldAcquiresBeforeTheHandshakeAndReleasesWhenThePeerCloses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	acquired := make(chan struct{})
	released := make(chan struct{})
	engine := gin.New()
	engine.GET("/attach", func(c *gin.Context) {
		ws.Hold(c, func() func() {
			close(acquired)
			return func() { close(released) }
		})
	})
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/attach", nil)
	require.NoError(t, err)
	_ = resp.Body.Close()
	<-acquired
	select {
	case <-released:
		t.Fatal("released while the peer is still connected")
	default:
	}

	require.NoError(t, conn.Close())
	<-released
}
