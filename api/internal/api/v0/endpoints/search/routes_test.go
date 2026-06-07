package search_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

type stubReader struct {
	path string
}

func (s stubReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{WorktreePath: s.path}, nil
}

func TestRegister_MountsBothRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "main.go"),
		[]byte("hello\n"),
		0o600,
	))
	r := gin.New()
	search.Register(
		r.Group("/v0"),
		enginesearch.New(),
		stubReader{path: root},
	)

	mounted := make(map[string]bool)
	for _, route := range r.Routes() {
		mounted[route.Path] = true
	}
	assert.True(t, mounted["/v0/workspaces/:wsId/search"])
	assert.True(t, mounted["/v0/workspaces/:wsId/search/replace"])

	req := httptest.NewRequest(
		http.MethodPost,
		"/v0/workspaces/ws1/search",
		bytes.NewBufferString(`{"query":"hello","caseSensitive":true}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}
