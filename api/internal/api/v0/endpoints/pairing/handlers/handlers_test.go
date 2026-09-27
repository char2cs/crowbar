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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/pairing/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type fakePairing struct {
	mintCode   string
	mintErr    error
	redeemErr  error
	redeemTok  string
	redeemName string
}

func (f *fakePairing) Mint(time.Time) (string, error) { return f.mintCode, f.mintErr }

func (f *fakePairing) Redeem(_ context.Context, code, name string, _ time.Time) (string, domain.Client, error) {
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

// newTestRouter mounts each handler method directly, the way routes.go's
// RegisterOpen/RegisterGuarded do in production — this package has no
// Register of its own (route mounting is the outer pairing package's job,
// covered by its own routes_test.go).
func newTestRouter(pairingUC *fakePairing, registry *fakeRegistry) *gin.Engine {
	r := gin.New()
	h := handlers.New(pairingUC, registry)
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
			Code string `json:"code"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "123456", body.Data.Code)
}

func TestMintCode_MintError_Returns500(t *testing.T) {
	r := newTestRouter(&fakePairing{mintErr: errors.New("entropy source unavailable")}, &fakeRegistry{})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pair/code", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestMintCode_OverTLS_Returns403 pins that minting must be refused for any
// request that arrived over TLS — only a plaintext (unix socket / loopback
// dev TCP) caller may mint.
func TestMintCode_OverTLS_Returns403(t *testing.T) {
	r := newTestRouter(&fakePairing{mintCode: "123456"}, &fakeRegistry{})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pair/code", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code, "expected 403 for a mint request over TLS")
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

func TestRevokeClient_MarksRevoked(t *testing.T) {
	registry := &fakeRegistry{clients: []domain.Client{{ID: "c1"}}}
	r := newTestRouter(&fakePairing{}, registry)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/clients/c1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "c1", registry.revoked)
}

func TestRevokeClient_UnknownID_Returns404(t *testing.T) {
	r := newTestRouter(&fakePairing{}, &fakeRegistry{})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/clients/ghost", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
