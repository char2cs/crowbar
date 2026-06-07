package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	searchhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

type stubReader struct {
	path   string
	locked bool
	err    error
}

func (s stubReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	if s.err != nil {
		return domain.Workspace{}, s.err
	}
	return domain.Workspace{
		WorktreePath: s.path,
		Locked:       s.locked,
	}, nil
}

func newRouter(
	t *testing.T,
	eng searchhandlers.SearchEngine,
	reader searchhandlers.WorkspaceReader,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := searchhandlers.New(eng, reader)
	rg := r.Group("/v0")
	rg.POST("/workspaces/:wsId/search", h.Search)
	rg.POST("/workspaces/:wsId/search/replace", h.Replace)
	return r
}

func seedFile(
	t *testing.T,
	root string,
	rel string,
	content string,
) {
	t.Helper()
	abs := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o600))
}

func doPost(
	t *testing.T,
	r *gin.Engine,
	path string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(
		http.MethodPost,
		path,
		bytes.NewBufferString(body),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type envelope struct {
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

func decodeEnvelope(
	t *testing.T,
	w *httptest.ResponseRecorder,
) envelope {
	t.Helper()
	var env envelope
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	return env
}

func TestSearch_NoEngine_503(t *testing.T) {
	r := newRouter(t, nil, stubReader{path: t.TempDir()})

	w := doPost(t, r, "/v0/workspaces/ws1/search", `{"query":"hello"}`)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	env := decodeEnvelope(t, w)
	assert.False(t, env.Success)
	assert.NotEmpty(t, env.Error)
}

func TestReplace_NoEngine_503(t *testing.T) {
	r := newRouter(t, nil, stubReader{path: t.TempDir()})

	w := doPost(
		t,
		r,
		"/v0/workspaces/ws1/search/replace",
		`{"query":"hello","replacement":"bye","scope":"all"}`,
	)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.False(t, decodeEnvelope(t, w).Success)
}

func TestSearch_WorkspaceNotFound_404(t *testing.T) {
	r := newRouter(
		t,
		enginesearch.New(),
		stubReader{err: errors.New("no row")},
	)

	w := doPost(t, r, "/v0/workspaces/missing/search", `{"query":"hello"}`)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.False(t, decodeEnvelope(t, w).Success)
}

func TestReplace_WorkspaceNotFound_404(t *testing.T) {
	r := newRouter(
		t,
		enginesearch.New(),
		stubReader{err: errors.New("no row")},
	)

	w := doPost(
		t,
		r,
		"/v0/workspaces/missing/search/replace",
		`{"query":"hello","replacement":"bye","scope":"all"}`,
	)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.False(t, decodeEnvelope(t, w).Success)
}
