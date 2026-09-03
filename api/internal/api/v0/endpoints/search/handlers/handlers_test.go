package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubEngine struct{}

func (stubEngine) Search(
	_ context.Context,
	_ string,
	_ enginesearch.SearchRequest,
) (enginesearch.SearchResponse, error) {
	return enginesearch.SearchResponse{}, nil
}

func (stubEngine) Replace(
	_ context.Context,
	_ string,
	_ enginesearch.ReplaceRequest,
	_ bool,
) error {
	return nil
}

type stubReader struct{}

func (stubReader) Get(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, WorktreePath: "/repo", Branch: "main"}, nil
}

func newRouter() *gin.Engine {
	return newRouterWith(stubEngine{}, stubReader{})
}

func newRouterWith(eng handlers.SearchEngine, r handlers.WorkspaceReader) *gin.Engine {
	router := gin.New()
	h := handlers.New(eng, r)
	rg := router.Group("/v0")
	rg.POST("/workspaces/:wsId/search", h.Search)
	rg.POST("/workspaces/:wsId/search/replace", h.Replace)
	return router
}

func do(
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

func TestSearchHandlers_HappyPath(
	t *testing.T,
) {
	r := newRouter()

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/search",
		map[string]any{"query": "fmt"})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodPost, "/v0/workspaces/ws1/search/replace",
		map[string]any{"query": "fmt", "replacement": "log"})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSearchHandlers_MissingQuery(
	t *testing.T,
) {
	r := newRouter()
	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/search", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// doRaw posts a raw, possibly-malformed body — unlike do (which json.Marshals
// a Go value, so it can never produce actually-invalid JSON), this is how the
// ShouldBindJSON decode-failure branch gets exercised.
func doRaw(
	r *gin.Engine,
	method, path, rawBody string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(rawBody)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestSearchHandlers_Search_BadJSON(
	t *testing.T,
) {
	r := newRouter()
	rec := doRaw(r, http.MethodPost, "/v0/workspaces/ws1/search", `{"query":`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSearchHandlers_Replace_BadJSON(
	t *testing.T,
) {
	r := newRouter()
	rec := doRaw(r, http.MethodPost, "/v0/workspaces/ws1/search/replace", `{"query":`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
