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
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
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

func newRouter() *gin.Engine {
	return newRouterWith(stubEngine{})
}

func newRouterWith(eng handlers.SearchEngine) *gin.Engine {
	router := gin.New()
	h := handlers.New(eng)
	rg := router.Group("/v0/chats/:chatId")
	rg.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, domain.Workspace{
			ID:           c.Param("chatId"),
			WorktreePath: "/repo",
			Branch:       "main",
			Provisioning: domain.WorkspaceProvisioned,
		})
		c.Next()
	})
	rg.POST("/search", h.Search)
	rg.POST("/search/replace", h.Replace)
	return router
}

func newUnscopedRouterWith(eng handlers.SearchEngine) *gin.Engine {
	router := gin.New()
	h := handlers.New(eng)
	rg := router.Group("/v0/chats/:chatId")
	rg.POST("/search", h.Search)
	rg.POST("/search/replace", h.Replace)
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

	rec := do(r, http.MethodPost, "/v0/chats/chat1/search",
		map[string]any{"query": "fmt"})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodPost, "/v0/chats/chat1/search/replace",
		map[string]any{"query": "fmt", "replacement": "log"})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSearchHandlers_MissingQuery(
	t *testing.T,
) {
	r := newRouter()
	rec := do(r, http.MethodPost, "/v0/chats/chat1/search", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

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
	rec := doRaw(r, http.MethodPost, "/v0/chats/chat1/search", `{"query":`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSearchHandlers_Replace_BadJSON(
	t *testing.T,
) {
	r := newRouter()
	rec := doRaw(r, http.MethodPost, "/v0/chats/chat1/search/replace", `{"query":`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
