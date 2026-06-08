package search_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search"
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
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	search.Register(r.Group("/v0"), stubEngine{}, stubReader{})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v0/workspaces/ws1/search"},
		{http.MethodPost, "/v0/workspaces/ws1/search/replace"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
