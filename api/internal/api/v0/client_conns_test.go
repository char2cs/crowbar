package v0

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// echoStream upgrades and echoes every message back, reporting the upgrade's
// outcome on upgraded.
func echoStream(
	upgraded chan<- error,
) gin.HandlerFunc {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	return func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		upgraded <- err
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			kind, msg, err := conn.ReadMessage()
			if err != nil || conn.WriteMessage(kind, msg) != nil {
				return
			}
		}
	}
}

// streamServer serves echoStream behind requireBearerAuth over TLS.
func streamServer(
	t *testing.T,
	clients clientAuthStore,
	conns *clientConns,
) (*httptest.Server, <-chan error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	upgraded := make(chan error, 8)
	r := gin.New()
	r.Use(requireBearerAuth(clients, conns))
	r.GET("/ws", echoStream(upgraded))
	srv := httptest.NewTLSServer(r)
	t.Cleanup(srv.Close)
	return srv, upgraded
}

func dialStream(
	t *testing.T,
	srv *httptest.Server,
	token string,
) (*websocket.Conn, error) {
	t.Helper()
	transport, ok := srv.Client().Transport.(*http.Transport)
	require.True(t, ok)
	dialer := websocket.Dialer{TLSClientConfig: transport.TLSClientConfig}
	url := "wss" + strings.TrimPrefix(srv.URL, "https") + "/ws"
	conn, resp, err := dialer.DialContext(t.Context(), url, http.Header{"Authorization": {"Bearer " + token}})
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, nil
}

// requireEcho proves conn is still live: a message round-trips.
func requireEcho(
	t *testing.T,
	conn *websocket.Conn,
) {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("ping")))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "ping", string(msg))
}

// requireClosed proves the server closed conn; the deadline only bounds a failure.
func requireClosed(
	t *testing.T,
	conn *websocket.Conn,
) {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	_, _, err := conn.ReadMessage()
	require.Error(t, err)
	var netErr net.Error
	assert.False(t, errors.As(err, &netErr) && netErr.Timeout(), "the stream must be closed, not merely quiet: %v", err)
}

func held(
	conns *clientConns,
) int {
	conns.mu.Lock()
	defer conns.mu.Unlock()
	n := 0
	for _, set := range conns.conns {
		n += len(set)
	}
	return n
}

func pairedStore(
	ids ...string,
) *stubClientStore {
	store := &stubClientStore{}
	for _, id := range ids {
		store.clients = append(store.clients, domain.Client{ID: id, TokenHash: pairing.HashToken("token-" + id)})
	}
	return store
}

// TestClientConns_DisconnectEndsOnlyThatClientsStreams proves Disconnect
// closes every stream the named client's token opened and no other client's.
func TestClientConns_DisconnectEndsOnlyThatClientsStreams(t *testing.T) {
	conns := newClientConns()
	srv, upgraded := streamServer(t, pairedStore("c1", "c2"), conns)
	first, err := dialStream(t, srv, "token-c1")
	require.NoError(t, err)
	second, err := dialStream(t, srv, "token-c1")
	require.NoError(t, err)
	other, err := dialStream(t, srv, "token-c2")
	require.NoError(t, err)
	for range 3 {
		require.NoError(t, <-upgraded)
	}
	require.Equal(t, 3, held(conns))

	conns.Disconnect("c1")

	requireClosed(t, first)
	requireClosed(t, second)
	requireEcho(t, other)
	assert.Equal(t, 1, held(conns), "only the other client's stream is still held")
}

// TestClientConns_ClosedStreamLeavesTheRegistry proves a stream that ends on
// its own is forgotten, so the registry is bounded by live streams.
func TestClientConns_ClosedStreamLeavesTheRegistry(t *testing.T) {
	conns := newClientConns()
	srv, upgraded := streamServer(t, pairedStore("c1"), conns)
	conn, err := dialStream(t, srv, "token-c1")
	require.NoError(t, err)
	require.NoError(t, <-upgraded)
	require.Equal(t, 1, held(conns))

	require.NoError(t, conn.Close())

	require.Eventually(t, func() bool { return held(conns) == 0 }, 10*time.Second, time.Millisecond)
}

// revokedAfterFirstLookup is a client revoked between the bearer guard's
// lookup and the upgrade's hijack.
type revokedAfterFirstLookup struct {
	clientAuthStore
	lookups atomic.Int32
}

func (s *revokedAfterFirstLookup) FindWhere(ctx context.Context, match domain.Client) ([]domain.Client, error) {
	found, err := s.clientAuthStore.FindWhere(ctx, match)
	if s.lookups.Add(1) == 1 {
		return found, err
	}
	revokedAt := time.Now()
	for i := range found {
		found[i].RevokedAt = &revokedAt
	}
	return found, err
}

// TestClientConns_RevokeDuringUpgradeRefusesTheStream proves a revoke landing
// after the guard's check but before the hijack registered the connection
// still ends it: the hijack re-reads the client once registered.
func TestClientConns_RevokeDuringUpgradeRefusesTheStream(t *testing.T) {
	conns := newClientConns()
	srv, upgraded := streamServer(t, &revokedAfterFirstLookup{clientAuthStore: pairedStore("c1")}, conns)

	_, err := dialStream(t, srv, "token-c1")

	require.Error(t, err, "the handshake must not complete for a client revoked mid-upgrade")
	require.Error(t, <-upgraded)
	assert.Equal(t, 0, held(conns))
}

// TestClientConns_PlaintextStreamIsNotTracked proves a stream over the local,
// unauthenticated transport is never registered: it has no client to revoke.
func TestClientConns_PlaintextStreamIsNotTracked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	conns := newClientConns()
	upgraded := make(chan error, 1)
	r := gin.New()
	r.Use(requireBearerAuth(pairedStore(), conns))
	r.GET("/ws", echoStream(upgraded))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn, resp, err := websocket.DefaultDialer.DialContext(t.Context(), "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, <-upgraded)

	requireEcho(t, conn)
	assert.Equal(t, 0, held(conns))
}
