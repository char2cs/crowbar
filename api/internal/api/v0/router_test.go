//go:build integration

package v0_test

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/core/logring"
)

// daemonEngine builds the production HTTP surface, so routes mounted on the
// engine itself (pprof) are walked alongside the /v0 group Register builds.
func daemonEngine(
	t *testing.T,
) *gin.Engine {
	t.Helper()
	return daemonEngineWithLogs(t, logring.New())
}

// daemonEngineWithLogs is daemonEngine serving ring on GET /v0/console/logs.
func daemonEngineWithLogs(
	t *testing.T,
	ring logring.Ring,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	daemon, err := crowbarapi.New(tc.app, tc.eng, nil, "", v0.WithLogs(ring))
	require.NoError(t, err)
	t.Cleanup(daemon.Close)
	engine, ok := daemon.Handler().(*gin.Engine)
	require.True(t, ok, "the api container serves a gin engine")
	return engine
}

// serveTLS runs one request through h as if it arrived on the TLS listener,
// the only one on which requireBearerAuth enforces anything.
func serveTLS(
	t *testing.T,
	h http.Handler,
	method, path, bearer, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.TLS = &tls.ConnectionState{}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// mintCode mints a code over the plaintext path, as `crowbar pair` does on the
// unix socket.
func mintCode(
	t *testing.T,
	h http.Handler,
) string {
	t.Helper()
	mint := httptest.NewRecorder()
	h.ServeHTTP(mint, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v0/pair/code", http.NoBody))
	require.Equal(t, http.StatusOK, mint.Code, mint.Body.String())
	var minted struct {
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(mint.Body.Bytes(), &minted))
	return minted.Data.Code
}

// redeemFrom posts code to the redeem route over TLS from the TCP peer remote.
func redeemFrom(
	t *testing.T,
	h http.Handler,
	remote, code string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v0/pair",
		strings.NewReader(`{"code":"`+code+`","name":"audit"}`))
	req.TLS = &tls.ConnectionState{}
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// pairClient mints a code and redeems it over TLS, returning the client's
// bearer token and id.
func pairClient(
	t *testing.T,
	h http.Handler,
) (string, string) {
	t.Helper()
	redeem := serveTLS(t, h, http.MethodPost, "/v0/pair", "", `{"code":"`+mintCode(t, h)+`","name":"audit"}`)
	require.Equal(t, http.StatusOK, redeem.Code, redeem.Body.String())
	var paired struct {
		Data struct {
			Token    string `json:"token"`
			ClientID string `json:"clientId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(redeem.Body.Bytes(), &paired))
	require.NotEmpty(t, paired.Data.Token)
	return paired.Data.Token, paired.Data.ClientID
}

// TestRouter_TLSNoAuth_OnlyPairingAndPprofAreOpen requests every route on the real
// engine: gin fixes middleware at registration, so a route added above the guard or
// beside mountDebug is only visible to a request. A guarded route must give the guard's 401.
func TestRouter_TLSNoAuth_OnlyPairingAndPprofAreOpen(t *testing.T) {
	engine := daemonEngine(t)

	open := map[string]int{
		"POST /v0/pair":           http.StatusBadRequest,
		"POST /v0/pair/code":      http.StatusForbidden,
		"GET /debug/pprof/*rest":  http.StatusForbidden,
		"POST /debug/pprof/*rest": http.StatusForbidden,
	}
	seenOpen := 0
	for _, rt := range engine.Routes() {
		route := rt.Method + " " + rt.Path
		segments := strings.Split(rt.Path, "/")
		for i, seg := range segments {
			if strings.HasPrefix(seg, ":") || strings.HasPrefix(seg, "*") {
				segments[i] = "x"
			}
		}
		rec := serveTLS(t, engine, rt.Method, strings.Join(segments, "/"), "", "")

		if want, isOpen := open[route]; isOpen {
			seenOpen++
			assert.Equalf(t, want, rec.Code, "%s over TLS with no token: %s", route, rec.Body.String())
			continue
		}
		assert.Equalf(t, http.StatusUnauthorized, rec.Code, "%s over TLS with no token: %s", route, rec.Body.String())
		assert.Containsf(t, rec.Body.String(), "client is not paired",
			"%s must be refused by the bearer guard, not by its own handler", route)
	}
	assert.Equal(t, len(open), seenOpen, "every open route must still be registered")
}

// TestRouter_TLS_HealthNeedsAPairedToken covers the token side of the guard: a
// paired client reads health, a token that was never issued does not, which is
// how a revoked host surfaces on the periodic probe.
func TestRouter_TLS_HealthNeedsAPairedToken(t *testing.T) {
	engine := daemonEngine(t)

	token, _ := pairClient(t, engine)
	assert.Equal(t, http.StatusOK, serveTLS(t, engine, http.MethodGet, "/v0/health", token, "").Code,
		"health must answer a paired client over TLS")
	assert.Equal(t, http.StatusUnauthorized, serveTLS(t, engine, http.MethodGet, "/v0/health", "never-issued", "").Code,
		"health must refuse an unknown token over TLS")
}

// TestRouter_TLS_PairRedeemIsLimitedPerSource proves the redeem limit is per TCP
// peer: a sixth redeem from one source within the window is refused, while
// another source's redeem still reaches the outstanding code.
func TestRouter_TLS_PairRedeemIsLimitedPerSource(t *testing.T) {
	engine := daemonEngine(t)
	code := mintCode(t, engine)
	wrong := "000000"
	if code == wrong {
		wrong = "000001"
	}
	const attacker = "203.0.113.9:40000"

	for i := range 5 {
		require.Equalf(t, http.StatusUnauthorized, redeemFrom(t, engine, attacker, wrong).Code, "guess %d", i)
	}
	assert.Equal(t, http.StatusTooManyRequests, redeemFrom(t, engine, attacker, code).Code,
		"the sixth redeem from one source within the window must be refused")
	rec := redeemFrom(t, engine, "198.51.100.7:50000", code)
	assert.Equal(t, http.StatusOK, rec.Code, "another source must still redeem the valid code: %s", rec.Body.String())
}

// TestRouter_TLS_RevokeEndsTheClientsOpenStreams proves a revoke reaches the
// streams a client already holds: the revoked client's console-log WebSocket
// closes, while another paired client's keeps receiving new records.
func TestRouter_TLS_RevokeEndsTheClientsOpenStreams(t *testing.T) {
	ring := logring.New()
	engine := daemonEngineWithLogs(t, ring)
	srv := httptest.NewTLSServer(engine)
	t.Cleanup(srv.Close)

	revokedToken, revokedID := pairClient(t, engine)
	keptToken, _ := pairClient(t, engine)
	revoked := dialLogs(t, srv, revokedToken)
	kept := dialLogs(t, srv, keptToken)

	rec := serveTLS(t, engine, http.MethodDelete, "/v0/clients/"+revokedID, keptToken, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	slog.New(ring.Wrap(slog.NewTextHandler(io.Discard, nil))).Info("logged after the revoke")

	err := readUntilClosed(t, revoked)
	var netErr net.Error
	assert.False(t, errors.As(err, &netErr) && netErr.Timeout(),
		"the revoked client's stream must be closed, not merely quiet: %v", err)
	readUntilFrame(t, kept, "logged after the revoke")
}

// dialLogs opens GET /v0/console/logs over srv's TLS with token, returning once
// the stream's ready frame says it is following live records.
func dialLogs(
	t *testing.T,
	srv *httptest.Server,
	token string,
) *websocket.Conn {
	t.Helper()
	transport, ok := srv.Client().Transport.(*http.Transport)
	require.True(t, ok)
	dialer := websocket.Dialer{TLSClientConfig: transport.TLSClientConfig}
	url := "wss" + strings.TrimPrefix(srv.URL, "https") + "/v0/console/logs"
	conn, resp, err := dialer.DialContext(t.Context(), url, http.Header{"Authorization": {"Bearer " + token}})
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() { _ = conn.Close() })
	readUntilFrame(t, conn, `"type":"ready"`)
	return conn
}

// readUntilFrame reads frames until one contains want; the deadline only
// bounds a failure.
func readUntilFrame(
	t *testing.T,
	conn *websocket.Conn,
	want string,
) {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	for {
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err, "waiting for a frame containing %s", want)
		if strings.Contains(string(msg), want) {
			return
		}
	}
}

// readUntilClosed reads frames until the connection fails, returning why.
func readUntilClosed(
	t *testing.T,
	conn *websocket.Conn,
) error {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return err
		}
	}
}
