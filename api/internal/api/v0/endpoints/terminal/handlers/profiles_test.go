package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func terminalProfile(
	id string,
) domain.TerminalProfile {
	return domain.TerminalProfile{
		ID:              id,
		Name:            "prof-" + id,
		Shell:           "/bin/bash",
		StartupCommands: []string{"echo hi"},
	}
}

func TestListProfiles_EmptyIsNonNil(t *testing.T) {
	h := newHandlers(stubEngine{}, newStubProfiles(), stubWorkspace{})
	w := doReq(t, h.ListProfiles, http.MethodGet, "/v0/settings/terminal/profiles", nil, "")
	require.Equal(t, http.StatusOK, w.Code)

	env := decodeEnvelope(t, w.Body)
	assert.True(t, env.Success)
	var list []domain.TerminalProfile
	require.NoError(t, json.Unmarshal(env.Data, &list))
	assert.NotNil(t, list)
	assert.Empty(t, list)
}

func TestListProfiles_StoreError(t *testing.T) {
	store := newStubProfiles()
	store.failAll = true
	h := newHandlers(stubEngine{}, store, stubWorkspace{})
	w := doReq(t, h.ListProfiles, http.MethodGet, "/v0/settings/terminal/profiles", nil, "")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestCreateProfile_AssignsID(t *testing.T) {
	store := newStubProfiles()
	h := newHandlers(stubEngine{}, store, stubWorkspace{})
	w := doReq(t, h.CreateProfile, http.MethodPost, "/v0/settings/terminal/profiles", nil, `{"name":"new","shell":"/bin/zsh"}`)
	require.Equal(t, http.StatusCreated, w.Code)

	env := decodeEnvelope(t, w.Body)
	var got domain.TerminalProfile
	require.NoError(t, json.Unmarshal(env.Data, &got))
	assert.NotEmpty(t, got.ID)
	assert.Equal(t, "/bin/zsh", got.Shell)
	assert.NotNil(t, got.StartupCommands)
}

func TestCreateProfile_KeepsProvidedID(t *testing.T) {
	store := newStubProfiles()
	h := newHandlers(stubEngine{}, store, stubWorkspace{})
	w := doReq(t, h.CreateProfile, http.MethodPost, "/v0/settings/terminal/profiles", nil, `{"id":"fixed","name":"n"}`)
	require.Equal(t, http.StatusCreated, w.Code)
	_, ok := store.items["fixed"]
	assert.True(t, ok)
}

func TestCreateProfile_BadBody(t *testing.T) {
	h := newHandlers(stubEngine{}, newStubProfiles(), stubWorkspace{})
	w := doReq(t, h.CreateProfile, http.MethodPost, "/v0/settings/terminal/profiles", nil, `{`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateProfile_StoreError(t *testing.T) {
	store := newStubProfiles()
	store.failAll = true
	h := newHandlers(stubEngine{}, store, stubWorkspace{})
	w := doReq(t, h.CreateProfile, http.MethodPost, "/v0/settings/terminal/profiles", nil, `{"name":"n"}`)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetProfile_HappyPath(t *testing.T) {
	store := newStubProfiles()
	require.NoError(t, store.Save(context.Background(), terminalProfile("p1")))
	h := newHandlers(stubEngine{}, store, stubWorkspace{})
	w := doReq(t, h.GetProfile, http.MethodGet, "/v0/settings/terminal/profiles/p1", gin.Params{{Key: "id", Value: "p1"}}, "")
	require.Equal(t, http.StatusOK, w.Code)

	env := decodeEnvelope(t, w.Body)
	var got domain.TerminalProfile
	require.NoError(t, json.Unmarshal(env.Data, &got))
	assert.Equal(t, "p1", got.ID)
}

func TestGetProfile_NotFound(t *testing.T) {
	h := newHandlers(stubEngine{}, newStubProfiles(), stubWorkspace{})
	w := doReq(t, h.GetProfile, http.MethodGet, "/v0/settings/terminal/profiles/ghost", gin.Params{{Key: "id", Value: "ghost"}}, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetProfile_StoreError(t *testing.T) {
	store := newStubProfiles()
	store.failAll = true
	h := newHandlers(stubEngine{}, store, stubWorkspace{})
	w := doReq(t, h.GetProfile, http.MethodGet, "/v0/settings/terminal/profiles/p1", gin.Params{{Key: "id", Value: "p1"}}, "")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdateProfile_HappyPath(t *testing.T) {
	store := newStubProfiles()
	require.NoError(t, store.Save(context.Background(), terminalProfile("p1")))
	h := newHandlers(stubEngine{}, store, stubWorkspace{})
	w := doReq(t, h.UpdateProfile, http.MethodPut, "/v0/settings/terminal/profiles/p1", gin.Params{{Key: "id", Value: "p1"}}, `{"name":"updated","shell":"/bin/fish"}`)
	require.Equal(t, http.StatusOK, w.Code)

	env := decodeEnvelope(t, w.Body)
	var got domain.TerminalProfile
	require.NoError(t, json.Unmarshal(env.Data, &got))
	assert.Equal(t, "p1", got.ID)
	assert.Equal(t, "/bin/fish", got.Shell)
}

func TestUpdateProfile_NotFound(t *testing.T) {
	h := newHandlers(stubEngine{}, newStubProfiles(), stubWorkspace{})
	w := doReq(t, h.UpdateProfile, http.MethodPut, "/v0/settings/terminal/profiles/ghost", gin.Params{{Key: "id", Value: "ghost"}}, `{"name":"x"}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateProfile_LookupError(t *testing.T) {
	store := newStubProfiles()
	store.failAll = true
	h := newHandlers(stubEngine{}, store, stubWorkspace{})
	w := doReq(t, h.UpdateProfile, http.MethodPut, "/v0/settings/terminal/profiles/p1", gin.Params{{Key: "id", Value: "p1"}}, `{"name":"x"}`)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdateProfile_BadBody(t *testing.T) {
	store := newStubProfiles()
	require.NoError(t, store.Save(context.Background(), terminalProfile("p1")))
	h := newHandlers(stubEngine{}, store, stubWorkspace{})
	w := doReq(t, h.UpdateProfile, http.MethodPut, "/v0/settings/terminal/profiles/p1", gin.Params{{Key: "id", Value: "p1"}}, `{`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateProfile_SaveError(t *testing.T) {
	store := &saveFailStore{stubProfiles: newStubProfiles()}
	require.NoError(t, store.stubProfiles.Save(context.Background(), terminalProfile("p1")))
	h := newHandlers(stubEngine{}, store, stubWorkspace{})
	w := doReq(t, h.UpdateProfile, http.MethodPut, "/v0/settings/terminal/profiles/p1", gin.Params{{Key: "id", Value: "p1"}}, `{"name":"x"}`)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestDeleteProfile_HappyPath(t *testing.T) {
	store := newStubProfiles()
	require.NoError(t, store.Save(context.Background(), terminalProfile("p1")))
	h := newHandlers(stubEngine{}, store, stubWorkspace{})
	w := doReq(t, h.DeleteProfile, http.MethodDelete, "/v0/settings/terminal/profiles/p1", gin.Params{{Key: "id", Value: "p1"}}, "")
	assert.Equal(t, http.StatusNoContent, w.Code)
	_, ok := store.items["p1"]
	assert.False(t, ok)
}

func TestDeleteProfile_NotFound(t *testing.T) {
	h := newHandlers(stubEngine{}, newStubProfiles(), stubWorkspace{})
	w := doReq(t, h.DeleteProfile, http.MethodDelete, "/v0/settings/terminal/profiles/ghost", gin.Params{{Key: "id", Value: "ghost"}}, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteProfile_LookupError(t *testing.T) {
	store := newStubProfiles()
	store.failAll = true
	h := newHandlers(stubEngine{}, store, stubWorkspace{})
	w := doReq(t, h.DeleteProfile, http.MethodDelete, "/v0/settings/terminal/profiles/p1", gin.Params{{Key: "id", Value: "p1"}}, "")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestDeleteProfile_DeleteError(t *testing.T) {
	store := &deleteFailStore{stubProfiles: newStubProfiles()}
	require.NoError(t, store.Save(context.Background(), terminalProfile("p1")))
	h := newHandlers(stubEngine{}, store, stubWorkspace{})
	w := doReq(t, h.DeleteProfile, http.MethodDelete, "/v0/settings/terminal/profiles/p1", gin.Params{{Key: "id", Value: "p1"}}, "")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
