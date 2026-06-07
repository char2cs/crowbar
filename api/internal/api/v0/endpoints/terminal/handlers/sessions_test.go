package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

func TestCreateSession_HappyPath(t *testing.T) {
	profiles := newStubProfiles()
	h := newHandlers(stubEngine{sid: "sess-1"}, profiles, stubWorkspace{path: "/tmp/wt"})
	w := doReq(t, h.CreateSession, http.MethodPost, "/v0/workspaces/w1/terminals", gin.Params{{Key: "wsId", Value: "w1"}}, "{}")
	require.Equal(t, http.StatusCreated, w.Code)

	env := decodeEnvelope(t, w.Body)
	assert.True(t, env.Success)
	var data struct {
		SessionID string `json:"sessionId"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, "sess-1", data.SessionID)
}

func TestCreateSession_WithProfile(t *testing.T) {
	profiles := newStubProfiles()
	require.NoError(t, profiles.Save(context.Background(), terminalProfile("p1")))
	h := newHandlers(stubEngine{sid: "sess-p"}, profiles, stubWorkspace{path: "/tmp/wt"})
	w := doReq(t, h.CreateSession, http.MethodPost, "/v0/workspaces/w1/terminals", gin.Params{{Key: "wsId", Value: "w1"}}, `{"profileId":"p1"}`)
	require.Equal(t, http.StatusCreated, w.Code)
}

func TestCreateSession_ProfileStoreError(t *testing.T) {
	store := newStubProfiles()
	store.failAll = true
	h := newHandlers(stubEngine{sid: "sess-err"}, store, stubWorkspace{path: "/tmp"})
	w := doReq(t, h.CreateSession, http.MethodPost, "/v0/workspaces/w1/terminals", gin.Params{{Key: "wsId", Value: "w1"}}, `{"profileId":"missing"}`)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestCreateSession_UnknownProfile_404(t *testing.T) {
	h := newHandlers(stubEngine{sid: "x"}, newStubProfiles(), stubWorkspace{path: "/tmp"})
	w := doReq(t, h.CreateSession, http.MethodPost, "/v0/workspaces/w1/terminals", gin.Params{{Key: "wsId", Value: "w1"}}, `{"profileId":"ghost"}`)
	require.Equal(t, http.StatusNotFound, w.Code)
	env := decodeEnvelope(t, w.Body)
	assert.False(t, env.Success)
	assert.Equal(t, "profile not found", env.Error)
}

func TestCreateSession_EmptyProfileDefault_201(t *testing.T) {
	h := newHandlers(stubEngine{sid: "sess-default"}, newStubProfiles(), stubWorkspace{path: "/tmp"})
	w := doReq(t, h.CreateSession, http.MethodPost, "/v0/workspaces/w1/terminals", gin.Params{{Key: "wsId", Value: "w1"}}, `{"profileId":""}`)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestCreateSession_WorkspaceNotFound(t *testing.T) {
	h := newHandlers(stubEngine{sid: "x"}, newStubProfiles(), stubWorkspace{fail: true})
	w := doReq(t, h.CreateSession, http.MethodPost, "/v0/workspaces/ghost/terminals", gin.Params{{Key: "wsId", Value: "ghost"}}, "{}")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreateSession_WorkspaceReaderError_500(t *testing.T) {
	h := newHandlers(stubEngine{sid: "x"}, newStubProfiles(), stubWorkspace{err: errors.New("db down")})
	w := doReq(t, h.CreateSession, http.MethodPost, "/v0/workspaces/w1/terminals", gin.Params{{Key: "wsId", Value: "w1"}}, "{}")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestCreateSession_BadBody(t *testing.T) {
	h := newHandlers(stubEngine{sid: "x"}, newStubProfiles(), stubWorkspace{path: "/tmp"})
	w := doReq(t, h.CreateSession, http.MethodPost, "/v0/workspaces/w1/terminals", gin.Params{{Key: "wsId", Value: "w1"}}, `{"profileId":`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateSession_EmptyBodyOK(t *testing.T) {
	h := newHandlers(stubEngine{sid: "sess-empty"}, newStubProfiles(), stubWorkspace{path: "/tmp"})
	w := doReq(t, h.CreateSession, http.MethodPost, "/v0/workspaces/w1/terminals", gin.Params{{Key: "wsId", Value: "w1"}}, "")
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestCreateSession_EngineError(t *testing.T) {
	h := newHandlers(stubEngine{createErr: errors.New("spawn failed")}, newStubProfiles(), stubWorkspace{path: "/tmp"})
	w := doReq(t, h.CreateSession, http.MethodPost, "/v0/workspaces/w1/terminals", gin.Params{{Key: "wsId", Value: "w1"}}, "{}")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestKillSession_HappyPath(t *testing.T) {
	h := newHandlers(stubEngine{}, newStubProfiles(), stubWorkspace{})
	w := doReq(t, h.KillSession, http.MethodDelete, "/v0/terminals/s1", gin.Params{{Key: "sessionId", Value: "s1"}}, "")
	require.Equal(t, http.StatusOK, w.Code)

	env := decodeEnvelope(t, w.Body)
	assert.True(t, env.Success)
	var data struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, "s1", data.ID)
}

func TestKillSession_NotFound(t *testing.T) {
	h := newHandlers(stubEngine{killErr: engineterminal.ErrSessionNotFound}, newStubProfiles(), stubWorkspace{})
	w := doReq(t, h.KillSession, http.MethodDelete, "/v0/terminals/ghost", gin.Params{{Key: "sessionId", Value: "ghost"}}, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestKillSession_EngineError(t *testing.T) {
	h := newHandlers(stubEngine{killErr: errors.New("boom")}, newStubProfiles(), stubWorkspace{})
	w := doReq(t, h.KillSession, http.MethodDelete, "/v0/terminals/s1", gin.Params{{Key: "sessionId", Value: "s1"}}, "")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
