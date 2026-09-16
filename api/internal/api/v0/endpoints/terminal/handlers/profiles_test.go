package handlers_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// notFoundProfiles reports every id as absent, exercising the 404 guards in
// UpdateProfile/DeleteProfile that stubProfiles (which always finds a row)
// never reaches.
type notFoundProfiles struct {
	stubProfiles
}

func (notFoundProfiles) FindByKey(
	_ context.Context,
	_ string,
) (*domain.TerminalProfile, error) {
	return nil, nil
}

// deleteErrProfiles finds the row fine but fails the actual delete, isolating
// DeleteProfile's own error branch from its FindByKey guard.
type deleteErrProfiles struct {
	stubProfiles
}

func (deleteErrProfiles) Delete(
	_ context.Context,
	_ string,
) error {
	return errors.New("disk full")
}

func newProfilesRouter(
	store handlers.ProfileStore,
) *gin.Engine {
	r := gin.New()
	h := handlers.New(stubEngine{}, store, &spyBroadcaster{})
	rg := r.Group("/v0")
	rg.GET("/settings/terminal/profiles", h.ListProfiles)
	rg.GET("/settings/terminal/profiles/:id", h.GetProfile)
	rg.POST("/settings/terminal/profiles", h.CreateProfile)
	rg.PUT("/settings/terminal/profiles/:id", h.UpdateProfile)
	rg.DELETE("/settings/terminal/profiles/:id", h.DeleteProfile)
	return r
}

// doRaw posts a raw, possibly-malformed body — unlike do/doTerminal, which
// json.Marshal a Go value and so can never produce actually-invalid JSON.
func doRaw(
	r *gin.Engine,
	method, path string,
	body []byte,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestCreateProfile_400OnMalformedBody(t *testing.T) {
	r := newProfilesRouter(stubProfiles{})

	rec := doRaw(r, http.MethodPost, "/v0/settings/terminal/profiles", []byte("{not json"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUpdateProfile_404OnUnknownProfile(t *testing.T) {
	r := newProfilesRouter(notFoundProfiles{})

	rec := doRaw(r, http.MethodPut, "/v0/settings/terminal/profiles/ghost",
		[]byte(`{"name":"x"}`))

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestUpdateProfile_400OnMalformedBody(t *testing.T) {
	r := newProfilesRouter(stubProfiles{})

	rec := doRaw(r, http.MethodPut, "/v0/settings/terminal/profiles/p1", []byte("{not json"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDeleteProfile_404OnUnknownProfile(t *testing.T) {
	r := newProfilesRouter(notFoundProfiles{})

	rec := doRaw(r, http.MethodDelete, "/v0/settings/terminal/profiles/ghost", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteProfile_500WhenTheStoreDeleteFails(t *testing.T) {
	r := newProfilesRouter(deleteErrProfiles{})

	rec := doRaw(r, http.MethodDelete, "/v0/settings/terminal/profiles/p1", nil)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
