package files_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files"
	fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubFiles struct{}

func (stubFiles) Tree(
	_ context.Context,
	_ string,
	_ string,
	_ fileusecase.FileStatusProvider,
) ([]domain.FileNode, error) {
	return nil, nil
}

func (stubFiles) ReadContent(
	_ context.Context,
	_ string,
	_ string,
) (domain.FileContent, error) {
	return domain.FileContent{}, nil
}

func (stubFiles) WriteContent(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubFiles) CreateFile(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubFiles) CreateDir(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubFiles) Rename(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubFiles) Delete(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	files.Register(r.Group("/v0"), stubFiles{}, func(_ *gin.Context) {})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/workspaces/ws1/files/tree"},
		{http.MethodGet, "/v0/workspaces/ws1/files/content"},
		{http.MethodPut, "/v0/workspaces/ws1/files/content"},
		{http.MethodPost, "/v0/workspaces/ws1/files"},
		{http.MethodPatch, "/v0/workspaces/ws1/files"},
		{http.MethodDelete, "/v0/workspaces/ws1/files"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
