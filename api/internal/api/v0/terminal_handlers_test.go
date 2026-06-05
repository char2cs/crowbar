package v0_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// seedWorkspace creates a workspace in the repo and returns its ID.
func seedWorkspace(
	t *testing.T,
	tc testContainers,
	id string,
) string {
	t.Helper()
	dir := t.TempDir()
	_, err := tc.app.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{
			ID:           id,
			RepoID:       "r1",
			ProjectID:    "p1",
			WorktreePath: dir,
		},
		time.Now(),
	)
	require.NoError(t, err)
	return id
}

func newRouter(
	t *testing.T,
	tc testContainers,
) (*v0.Container, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	return c, r
}

// TestCreateTerminal_HappyPath creates a workspace and opens a terminal session.
func TestCreateTerminal_HappyPath(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)
	wsID := seedWorkspace(t, tc, "ws-term-1")

	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/"+wsID+"/terminals", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var got map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.NotEmpty(t, got["sessionId"])

	// Clean up the session.
	sid := got["sessionId"].(string)
	req2 := httptest.NewRequest(http.MethodDelete, "/v0/terminals/"+sid, nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusNoContent, w2.Code)
}

func TestCreateTerminal_WorkspaceNotFound(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)

	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/no-such-ws/terminals", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreateTerminal_NoEngine_503(t *testing.T) {
	tc := newApp(t)
	tc.eng.Terminal = nil
	_, r := newRouter(t, tc)

	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/ws1/terminals", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestKillTerminal_NotFound(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)

	req := httptest.NewRequest(http.MethodDelete, "/v0/terminals/ghost-session", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTerminalWS_SessionNotFound(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// Non-WS request to the WS endpoint without upgrading returns 404.
	req := httptest.NewRequest(http.MethodGet, "/v0/ws/terminals/no-such-session", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- Profile CRUD ---

func TestProfileCRUD(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)

	// List — starts empty.
	req := httptest.NewRequest(http.MethodGet, "/v0/settings/terminal/profiles", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var list []domain.TerminalProfile
	require.NoError(t, json.NewDecoder(w.Body).Decode(&list))
	assert.Empty(t, list)

	// Create.
	prof := domain.TerminalProfile{Shell: "/bin/bash", Name: "test-profile"}
	profBody, _ := json.Marshal(prof)
	req = httptest.NewRequest(http.MethodPost, "/v0/settings/terminal/profiles", bytes.NewReader(profBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var created domain.TerminalProfile
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "/bin/bash", created.Shell)

	// Get.
	req = httptest.NewRequest(http.MethodGet, "/v0/settings/terminal/profiles/"+created.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Update.
	updated := created
	updated.Shell = "/bin/zsh"
	updBody, _ := json.Marshal(updated)
	req = httptest.NewRequest(http.MethodPut, "/v0/settings/terminal/profiles/"+created.ID, bytes.NewReader(updBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var updatedResp domain.TerminalProfile
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updatedResp))
	assert.Equal(t, "/bin/zsh", updatedResp.Shell)

	// Delete.
	req = httptest.NewRequest(http.MethodDelete, "/v0/settings/terminal/profiles/"+created.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Get after delete — 404.
	req = httptest.NewRequest(http.MethodGet, "/v0/settings/terminal/profiles/"+created.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetProfile_NotFound(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/terminal/profiles/ghost", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateProfile_NotFound(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)

	body, _ := json.Marshal(domain.TerminalProfile{Shell: "/bin/sh"})
	req := httptest.NewRequest(http.MethodPut, "/v0/settings/terminal/profiles/ghost", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteProfile_NotFound(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)

	req := httptest.NewRequest(http.MethodDelete, "/v0/settings/terminal/profiles/ghost", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestTerminalWS_Connect upgrades to a real WebSocket, sends a message, then closes.
func TestTerminalWS_Connect(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	wsID := seedWorkspace(t, tc, "ws-term-ws")

	// Create a session.
	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/"+wsID+"/terminals", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	sid := created["sessionId"].(string)

	// Connect via WebSocket.
	wsURL := "ws" + srv.URL[len("http"):] + "/v0/ws/terminals/" + sid
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	// Close the connection — Attach will unblock and the session stays alive.
	_ = conn.Close()

	// Kill the session.
	req2 := httptest.NewRequest(http.MethodDelete, "/v0/terminals/"+sid, nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusNoContent, w2.Code)
}
