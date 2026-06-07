package repos_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubStore struct{}

func (stubStore) FindAll(
	_ context.Context,
) ([]domain.Repository, error) {
	return nil, nil
}

func (stubStore) FindByKey(
	_ context.Context,
	_ string,
) (*domain.Repository, error) {
	return &domain.Repository{ID: "r1"}, nil
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	repos.Register(r.Group("/v0"), stubStore{})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/repos"},
		{http.MethodGet, "/v0/repos/r1"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
