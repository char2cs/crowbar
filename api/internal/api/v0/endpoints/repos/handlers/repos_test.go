package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type fakeStore struct {
	all     []domain.Repository
	allErr  error
	byKey   *domain.Repository
	byKeErr error
}

func (f *fakeStore) FindAll(
	_ context.Context,
) ([]domain.Repository, error) {
	return f.all, f.allErr
}

func (f *fakeStore) FindByKey(
	_ context.Context,
	_ string,
) (*domain.Repository, error) {
	return f.byKey, f.byKeErr
}

func newRouter(
	store repohandlers.Store,
) *gin.Engine {
	r := gin.New()
	h := repohandlers.New(store)
	rg := r.Group("/v0")
	rg.GET("/repos", h.List)
	rg.GET("/repos/:id", h.Detail)
	return r
}

func do(
	r *gin.Engine,
	target string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, http.NoBody)
	r.ServeHTTP(rec, req)
	return rec
}

func TestListSuccess(
	t *testing.T,
) {
	store := &fakeStore{
		all: []domain.Repository{
			{ID: "r1", ProjectID: "p1", Name: "alpha"},
			{ID: "r2", ProjectID: "p2", Name: "beta"},
		},
	}
	rec := do(newRouter(store), "/v0/repos")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Len(t, body.Data, 2)
}

func TestListFilteredByProject(
	t *testing.T,
) {
	store := &fakeStore{
		all: []domain.Repository{
			{ID: "r1", ProjectID: "p1"},
			{ID: "r2", ProjectID: "p2"},
		},
	}
	rec := do(newRouter(store), "/v0/repos?projectId=p2")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "r2", body.Data[0].ID)
}

func TestListError(
	t *testing.T,
) {
	rec := do(newRouter(&fakeStore{allErr: errors.New("db down")}), "/v0/repos")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestDetailSuccess(
	t *testing.T,
) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r9", Name: "gamma"}}
	rec := do(newRouter(store), "/v0/repos/r9")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "r9", body.Data.ID)
}

func TestDetailNotFound(
	t *testing.T,
) {
	rec := do(newRouter(&fakeStore{byKey: nil}), "/v0/repos/missing")

	assert.Equal(t, http.StatusNotFound, rec.Code)
	var body struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Success)
	assert.NotEmpty(t, body.Error)
}

func TestDetailError(
	t *testing.T,
) {
	rec := do(newRouter(&fakeStore{byKeErr: errors.New("boom")}), "/v0/repos/r1")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
