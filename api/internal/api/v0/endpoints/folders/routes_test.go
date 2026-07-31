package folders_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/folders"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app/usecases/folder"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubUsecase struct{}

func (stubUsecase) Create(
	_ context.Context,
	_ folder.CreateInput,
) (domain.Folder, []domain.Folder, error) {
	return domain.Folder{ID: "f1"}, nil, nil
}

func (stubUsecase) Rename(
	_ context.Context,
	_ string,
	_ string,
) (domain.Folder, error) {
	return domain.Folder{ID: "f1"}, nil
}

func (stubUsecase) Move(
	_ context.Context,
	_ string,
	_ folder.MoveInput,
) (domain.Folder, []domain.Folder, error) {
	return domain.Folder{ID: "f1"}, nil, nil
}

func (stubUsecase) Delete(
	_ context.Context,
	_ string,
) ([]domain.Folder, error) {
	return nil, nil
}

func (stubUsecase) ListInRepo(
	_ context.Context,
	_ string,
	_ string,
) ([]domain.Folder, error) {
	return nil, nil
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	// folders.Register mounts on the repo-scoped group, so build the hierarchical
	// prefix to mirror the production router chain.
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	noopWS := func(_ *gin.Context) {}
	folders.Register(repoScoped, stubUsecase{}, func(dto.FolderDTO) {}, noopWS, ws.DualServe)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/projects/p1/repos/r1/folders"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/folders"},
		{http.MethodPatch, "/v0/projects/p1/repos/r1/folders/f1"},
		{http.MethodDelete, "/v0/projects/p1/repos/r1/folders/f1"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
