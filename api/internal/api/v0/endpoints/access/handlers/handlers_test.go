package handlers_test

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/access/handlers"
	"github.com/char2cs/crowbar/api/internal/core/exposure"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type fakeAccess struct {
	st         exposure.Status
	enableWith string
	enableErr  error
	disables   int
}

func (f *fakeAccess) Status() exposure.Status { return f.st }

func (f *fakeAccess) Enable(address string) (exposure.Status, error) {
	f.enableWith = address
	if f.enableErr != nil {
		return f.st, f.enableErr
	}
	f.st.Enabled, f.st.Address = true, address
	return f.st, nil
}

func (f *fakeAccess) Disable() (exposure.Status, error) {
	f.disables++
	f.st.Enabled = false
	return f.st, nil
}

func serve(t *testing.T, a *fakeAccess, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	return serveOver(t, a, method, body, nil)
}

func serveOver(t *testing.T, a *fakeAccess, method, body string, state *tls.ConnectionState) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	h := handlers.New(a)
	r.GET("/access", h.Get)
	r.PUT("/access", h.Put)
	req := httptest.NewRequestWithContext(t.Context(), method, "/access", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = state
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestGet_ReportsTheIdentityWhileClosed(t *testing.T) {
	rec := serve(t, &fakeAccess{st: exposure.Status{Address: "0.0.0.0:3737", Fingerprint: "aaaa bbbb"}}, http.MethodGet, "")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"success":true,"data":{"enabled":false,"address":"0.0.0.0:3737","reachable":[],"fingerprint":"aaaa bbbb"}}`, rec.Body.String())
}

func TestPut_OnOpensTheAddress(t *testing.T) {
	a := &fakeAccess{}
	rec := serve(t, a, http.MethodPut, `{"enabled":true,"address":"0.0.0.0:4000"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "0.0.0.0:4000", a.enableWith)
	assert.Contains(t, rec.Body.String(), `"enabled":true`)
}

func TestPut_OffClosesWithoutAnAddress(t *testing.T) {
	a := &fakeAccess{st: exposure.Status{Enabled: true}}
	rec := serve(t, a, http.MethodPut, `{"enabled":false}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, a.disables)
	assert.Contains(t, rec.Body.String(), `"enabled":false`)
}

func TestPut_ARefusedAddressIs400(t *testing.T) {
	a := &fakeAccess{enableErr: errors.New("exposure: binds loopback only")}
	rec := serve(t, a, http.MethodPut, `{"enabled":true,"address":"127.0.0.1:1"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "loopback")
}

func TestPut_MalformedJSONIs400(t *testing.T) {
	assert.Equal(t, http.StatusBadRequest, serve(t, &fakeAccess{}, http.MethodPut, "nope").Code)
}

// A paired device must not be able to turn device connections off (or move
// them): only the owner at this computer decides who may connect at all.
func TestRegression_Put_RefusesAPairedRemoteDevice(t *testing.T) {
	for _, body := range []string{`{"enabled":false}`, `{"enabled":true,"address":"0.0.0.0:4000"}`} {
		a := &fakeAccess{st: exposure.Status{Enabled: true}}
		rec := serveOver(t, a, http.MethodPut, body, &tls.ConnectionState{})

		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "device connections can only be changed from this computer")
		assert.Zero(t, a.disables)
		assert.Empty(t, a.enableWith)
	}
}

func TestGet_AllowsAPairedRemoteDevice(t *testing.T) {
	rec := serveOver(t, &fakeAccess{st: exposure.Status{Fingerprint: "aaaa"}}, http.MethodGet, "", &tls.ConnectionState{})

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "aaaa")
}
