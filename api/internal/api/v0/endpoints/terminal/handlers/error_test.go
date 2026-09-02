package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type errProfiles struct{}

func (errProfiles) FindAll(_ context.Context) ([]domain.TerminalProfile, error) {
	return nil, errors.New("db down")
}

func (errProfiles) FindByKey(_ context.Context, _ string) (*domain.TerminalProfile, error) {
	return nil, errors.New("db down")
}

func (errProfiles) Save(_ context.Context, _ domain.TerminalProfile) error {
	return errors.New("db down")
}

func (errProfiles) Delete(_ context.Context, _ string) error { return errors.New("db down") }

func newErrRouter() *gin.Engine {
	r := gin.New()
	h := handlers.New(stubEngine{}, errProfiles{}, &spyBroadcaster{})
	mountSessions(r, h)
	rg := r.Group("/v0")
	rg.GET("/settings/terminal/profiles", h.ListProfiles)
	rg.GET("/settings/terminal/profiles/:id", h.GetProfile)
	rg.POST("/settings/terminal/profiles", h.CreateProfile)
	rg.PUT("/settings/terminal/profiles/:id", h.UpdateProfile)
	rg.DELETE("/settings/terminal/profiles/:id", h.DeleteProfile)
	return r
}

func newNilEngineRouter() *gin.Engine {
	r := gin.New()
	h := handlers.New(nil, stubProfiles{}, &spyBroadcaster{})
	mountSessions(r, h)
	return r
}

func doTerminal(
	r *gin.Engine,
	method, path string,
	body any,
) *httptest.ResponseRecorder {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestTerminalHandlers_NilEngine(
	t *testing.T,
) {
	r := newNilEngineRouter()

	rec := doTerminal(r, http.MethodPost, chatPath, nil)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	rec = doTerminal(r, http.MethodDelete, chatPath+"/sess1", nil)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestTerminalHandlers_ProfileErrors(
	t *testing.T,
) {
	r := newErrRouter()

	rec := doTerminal(r, http.MethodGet, "/v0/settings/terminal/profiles", nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = doTerminal(r, http.MethodGet, "/v0/settings/terminal/profiles/p1", nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = doTerminal(r, http.MethodPost, "/v0/settings/terminal/profiles",
		map[string]any{"name": "default"})
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = doTerminal(r, http.MethodPut, "/v0/settings/terminal/profiles/p1",
		map[string]any{"name": "x"})
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = doTerminal(r, http.MethodDelete, "/v0/settings/terminal/profiles/p1", nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
