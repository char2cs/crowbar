//go:build integration

package v0_test

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	daemon, err := crowbarapi.New(tc.app, tc.eng, nil, "", v0.WithLogs(logring.New()))
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

// pairedToken mints a code over the plaintext path (as `crowbar pair` does on
// the unix socket) and redeems it over TLS, returning the client's bearer token.
func pairedToken(
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

	redeem := serveTLS(t, h, http.MethodPost, "/v0/pair", "", `{"code":"`+minted.Data.Code+`","name":"audit"}`)
	require.Equal(t, http.StatusOK, redeem.Code, redeem.Body.String())
	var paired struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(redeem.Body.Bytes(), &paired))
	require.NotEmpty(t, paired.Data.Token)
	return paired.Data.Token
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

	token := pairedToken(t, engine)
	assert.Equal(t, http.StatusOK, serveTLS(t, engine, http.MethodGet, "/v0/health", token, "").Code,
		"health must answer a paired client over TLS")
	assert.Equal(t, http.StatusUnauthorized, serveTLS(t, engine, http.MethodGet, "/v0/health", "never-issued", "").Code,
		"health must refuse an unknown token over TLS")
}
