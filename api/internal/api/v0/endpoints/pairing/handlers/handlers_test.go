// Package handlers_test exercises the pairing HTTP surface end to end
// against gin's test harness, with a fake pairing.Usecase and client store —
// no real DB.
package handlers_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/pairing/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/core/exposure"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

const testFingerprint = "SHA256:aaaa bbbb cccc dddd eeee ffff"

// fakeAccess is the device listener's state a mint reads.
type fakeAccess struct{ st exposure.Status }

func (f fakeAccess) Status() exposure.Status { return f.st }

func listening() fakeAccess {
	return fakeAccess{st: exposure.Status{
		Enabled: true, Fingerprint: testFingerprint, Reachable: []string{"192.168.1.20:3737"},
	}}
}

type fakePairing struct {
	mintCode   string
	mintErr    error
	mints      int
	redeemErr  error
	redeemTok  string
	redeemName string
	redeems    int
}

func (f *fakePairing) Mint(time.Time) (string, error) {
	f.mints++
	return f.mintCode, f.mintErr
}

func (f *fakePairing) Redeem(_ context.Context, code, name string, _ time.Time) (string, domain.Client, error) {
	f.redeems++
	f.redeemName = name
	if f.redeemErr != nil {
		return "", domain.Client{}, f.redeemErr
	}
	return f.redeemTok, domain.Client{ID: "c1", Name: name}, nil
}

type fakeRegistry struct {
	clients     []domain.Client
	revoked     string
	findAllErr  error
	findByIDErr error
	saveErr     error
}

func (f *fakeRegistry) FindAll(context.Context) ([]domain.Client, error) {
	if f.findAllErr != nil {
		return nil, f.findAllErr
	}
	return f.clients, nil
}

func (f *fakeRegistry) Save(_ context.Context, c domain.Client) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.revoked = c.ID
	return nil
}

func (f *fakeRegistry) FindByKey(_ context.Context, id string) (*domain.Client, error) {
	if f.findByIDErr != nil {
		return nil, f.findByIDErr
	}
	for _, c := range f.clients {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, nil
}

// fakeStreams records which clients' streams were ended.
type fakeStreams struct {
	disconnected []string
}

func (f *fakeStreams) Disconnect(clientID string) {
	f.disconnected = append(f.disconnected, clientID)
}

// newTestRouter mounts each handler method directly, the way routes.go's
// RegisterOpen/RegisterGuarded do in production — this package has no
// Register of its own (route mounting is the outer pairing package's job,
// covered by its own routes_test.go).
func newTestRouter(pairingUC *fakePairing, registry *fakeRegistry) *gin.Engine {
	return newTestRouterWithStreams(pairingUC, registry, &fakeStreams{})
}

func newTestRouterWithStreams(pairingUC *fakePairing, registry *fakeRegistry, streams *fakeStreams) *gin.Engine {
	r := gin.New()
	h := handlers.New(pairingUC, registry, streams, listening())
	r.POST("/pair/code", h.MintCode)
	r.POST("/pair", h.Pair)
	r.GET("/clients", h.ListClients)
	r.DELETE("/clients/:id", h.RevokeClient)
	return r
}

func TestMintCode_ReturnsCode(t *testing.T) {
	r := newTestRouter(&fakePairing{mintCode: "123456"}, &fakeRegistry{})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pair/code", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Data struct {
			Code              string   `json:"code"`
			ServerFingerprint string   `json:"serverFingerprint"`
			Addresses         []string `json:"addresses"`
			ExpiresAt         string   `json:"expiresAt"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "123456", body.Data.Code)
	assert.Equal(t, []string{"192.168.1.20:3737"}, body.Data.Addresses)
	expires, err := time.Parse(time.RFC3339, body.Data.ExpiresAt)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(pairing.CodeTTL), expires, time.Minute)
	assert.Equal(t, testFingerprint, body.Data.ServerFingerprint,
		"the minted code must carry the daemon's fingerprint for the human trust-on-first-connect check")
}

func TestMintCode_MintError_Returns500(t *testing.T) {
	r := newTestRouter(&fakePairing{mintErr: errors.New("entropy source unavailable")}, &fakeRegistry{})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pair/code", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// A paired device reaches this route over TLS, past the bearer guard the router
// puts in front of it, so it can add another device from the remote's own UI.
func TestMintCode_OverTLS_Mints(t *testing.T) {
	r := newTestRouter(&fakePairing{mintCode: "123456"}, &fakeRegistry{})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pair/code", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// A code minted over TLS names the device that asked, so the owner can tell
// who let another device in; the code itself is never logged.
func TestRegression_MintCode_OverTLS_LogsWhoMinted(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	r := gin.New()
	r.Use(func(c *gin.Context) {
		libs.SetPairedClient(c, libs.PairedClient{ID: "c1", Name: "Ana's phone"})
	})
	r.POST("/pair/code", handlers.New(&fakePairing{mintCode: "123456"}, &fakeRegistry{}, &fakeStreams{}, listening()).MintCode)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pair/code", nil)
	req.TLS = &tls.ConnectionState{}
	r.ServeHTTP(httptest.NewRecorder(), req)

	assert.Contains(t, logs.String(), "client=c1")
	assert.Contains(t, logs.String(), "Ana's phone")
	assert.NotContains(t, logs.String(), "123456")
}

func TestPair_ValidCode_ReturnsToken(t *testing.T) {
	r := newTestRouter(&fakePairing{redeemTok: "sometoken"}, &fakeRegistry{})
	reqBody, _ := json.Marshal(map[string]string{"code": "123456", "name": "my-device"})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pair", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Data struct {
			Token    string `json:"token"`
			ClientID string `json:"clientId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "sometoken", body.Data.Token)
	assert.Equal(t, "c1", body.Data.ClientID)
}

func TestPair_BadCode_Returns401(t *testing.T) {
	r := newTestRouter(&fakePairing{redeemErr: pairing.ErrBadCode}, &fakeRegistry{})
	reqBody, _ := json.Marshal(map[string]string{"code": "000000", "name": "x"})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pair", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPair_InternalError_Returns500(t *testing.T) {
	r := newTestRouter(&fakePairing{redeemErr: errors.New("pairing: persist client: disk full")}, &fakeRegistry{})
	reqBody, _ := json.Marshal(map[string]string{"code": "123456", "name": "x"})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pair", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code, "a non-pairing-sentinel error must not be reported as an invalid code")
}

// TestRegression_MintCode_WhileNotListening_Refuses proves a daemon whose device
// listener is closed mints no code: no device can reach it to redeem one.
func TestRegression_MintCode_WhileNotListening_Refuses(t *testing.T) {
	pairingUC := &fakePairing{mintCode: "123456"}
	r := gin.New()
	r.POST("/pair/code", handlers.New(pairingUC, &fakeRegistry{}, &fakeStreams{}, fakeAccess{}).MintCode)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pair/code", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "device connections are off")
	assert.Zero(t, pairingUC.mints, "no code may be minted")
}

// postPair posts body to POST /pair on r.
func postPair(
	t *testing.T,
	r *gin.Engine,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pair", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestRegression_Pair_OversizedBodyIsRefused proves an anonymous caller cannot
// make the daemon read more than a pairing request's few bytes: the body is
// cut off at the cap and the code is never tried.
func TestRegression_Pair_OversizedBodyIsRefused(t *testing.T) {
	pairingUC := &fakePairing{redeemTok: "sometoken"}
	r := newTestRouter(pairingUC, &fakeRegistry{})

	rec := postPair(t, r, strings.Repeat(" ", 8<<10)+`{"code":"123456","name":"x"}`)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
	assert.Zero(t, pairingUC.redeems, "an oversized request must not reach the code")
}

// TestRegression_Pair_NameIsCappedAt64Runes proves the client name a paired
// device is listed under is bounded in characters, not bytes.
func TestRegression_Pair_NameIsCappedAt64Runes(t *testing.T) {
	pairingUC := &fakePairing{redeemTok: "sometoken"}
	r := newTestRouter(pairingUC, &fakeRegistry{})

	rec := postPair(t, r, `{"code":"123456","name":"`+strings.Repeat("é", 65)+`"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Zero(t, pairingUC.redeems, "an over-long name must not reach the code")

	rec = postPair(t, r, `{"code":"123456","name":"`+strings.Repeat("é", 64)+`"}`)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestPair_MalformedJSON_Returns400(t *testing.T) {
	r := newTestRouter(&fakePairing{}, &fakeRegistry{})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pair", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListClients_ReturnsAll(t *testing.T) {
	r := newTestRouter(&fakePairing{}, &fakeRegistry{clients: []domain.Client{{ID: "c1"}, {ID: "c2"}}})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/clients", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestListClients_StoreError_Returns500(t *testing.T) {
	r := newTestRouter(&fakePairing{}, &fakeRegistry{findAllErr: errors.New("store unavailable")})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/clients", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestRevokeClient_MarksRevokedAndEndsItsStreams proves a revoke both persists
// and ends the streams the client already holds, which no later request of
// its own would ever re-check.
func TestRevokeClient_MarksRevokedAndEndsItsStreams(t *testing.T) {
	registry := &fakeRegistry{clients: []domain.Client{{ID: "c1"}, {ID: "c2"}}}
	streams := &fakeStreams{}
	r := newTestRouterWithStreams(&fakePairing{}, registry, streams)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/clients/c1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "c1", registry.revoked)
	assert.Equal(t, []string{"c1"}, streams.disconnected)
}

// TestRevokeClient_SaveError_LeavesStreamsOpen proves a revoke that failed to
// persist does not end the streams of a client that is still paired.
func TestRevokeClient_SaveError_LeavesStreamsOpen(t *testing.T) {
	registry := &fakeRegistry{clients: []domain.Client{{ID: "c1"}}, saveErr: errors.New("disk full")}
	streams := &fakeStreams{}
	r := newTestRouterWithStreams(&fakePairing{}, registry, streams)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/clients/c1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Empty(t, streams.disconnected)
}

func TestRevokeClient_UnknownID_Returns404(t *testing.T) {
	r := newTestRouter(&fakePairing{}, &fakeRegistry{})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/clients/ghost", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
