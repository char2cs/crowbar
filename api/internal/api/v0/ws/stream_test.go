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

// streamServer serves ws.Stream over frames, each frame's text its own data and
// "end:<code>" closing with code.
func streamServer(t *testing.T, frames <-chan string, served chan<- struct{}) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/feed", func(c *gin.Context) {
		ws.Stream(c, frames, func(f string) ([]byte, int) {
			if code, ok := strings.CutPrefix(f, "end:"); ok {
				return []byte(f), map[string]int{"4000": 4000, "4001": 4001}[code]
			}
			return []byte(f), 0
		})
		if served != nil {
			close(served)
		}
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/feed"
}

func dialStream(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	return conn
}

func TestStream_WritesEachFrameThenClosesWithItsCode(t *testing.T) {
	frames := make(chan string, 4)
	frames <- "one"
	frames <- "two"
	frames <- "end:4000"
	conn := dialStream(t, streamServer(t, frames, nil))

	var got []string
	for range 3 {
		_, data, err := conn.ReadMessage()
		require.NoError(t, err)
		got = append(got, string(data))
	}
	_, _, err := conn.ReadMessage()

	assert.Equal(t, []string{"one", "two", "end:4000"}, got, "the closing frame is sent before the close")
	var closed *websocket.CloseError
	require.ErrorAs(t, err, &closed)
	assert.Equal(t, 4000, closed.Code)
}

func TestStream_ASourceThatEndsClosesTheSocket(t *testing.T) {
	frames := make(chan string)
	conn := dialStream(t, streamServer(t, frames, nil))

	close(frames)
	_, _, err := conn.ReadMessage()

	require.Error(t, err)
}

func TestStream_RefusesARequestThatIsNoUpgrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/feed", http.NoBody)

	ws.Stream(c, make(chan string), func(string) ([]byte, int) { return nil, 0 })

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestStream_ReturnsOnceTheClientGoes(t *testing.T) {
	served := make(chan struct{})
	conn, resp, err := websocket.DefaultDialer.Dial(streamServer(t, make(chan string), served), nil)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	require.NoError(t, err)

	require.NoError(t, conn.Close())

	<-served
}
