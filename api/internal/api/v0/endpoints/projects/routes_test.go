package projects_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubReader struct{}

func (stubReader) List(
	_ context.Context,
) ([]domain.Project, error) {
	return nil, nil
}

func (stubReader) Get(
	_ context.Context,
	_ string,
) (domain.Project, error) {
	return domain.Project{}, nil
}

type stubImporter struct{}

func (stubImporter) Import(
	_ context.Context,
	_ string,
	_ string,
) (domain.Project, error) {
	return domain.Project{}, nil
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	projects.Register(r.Group("/v0"), stubReader{}, stubImporter{})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/projects"},
		{http.MethodPost, "/v0/projects"},
		{http.MethodGet, "/v0/projects/abc"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
