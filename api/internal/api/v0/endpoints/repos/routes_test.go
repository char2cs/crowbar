package repos_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// stubOps replays nothing and starts nothing: the routes are only probed.
type stubOps struct{}

func (stubOps) Start(context.Context, operation.Spec, operation.Work) (domain.Operation, error) {
	return domain.Operation{}, nil
}

func (stubOps) Replay(context.Context, string, domain.OperationKind, domain.EntityRef) (domain.Operation, bool, error) {
	return domain.Operation{}, false, nil
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

func (stubStore) Save(
	_ context.Context,
	_ domain.Repository,
) error {
	return nil
}

func (stubStore) Delete(
	_ context.Context,
	_ string,
) error {
	return nil
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	// repos.Register mounts on the project-scoped group, so build the
	// hierarchical prefix to mirror the production router chain.
	projectScoped := r.Group("/v0/projects/:projectId")
	noopWS := func(_ *gin.Context) {}
	repos.Register(projectScoped, stubStore{}, nil, nil, nil, nil, nil, nil, nil, stubOps{}, func(dto.RepoDTO) {},
		func() (string, error) { return t.TempDir(), nil }, noopWS, ws.DualServe)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/projects/p1/repos"},
		{http.MethodGet, "/v0/projects/p1/repos/r1"},
		{http.MethodPatch, "/v0/projects/p1/repos/r1"},
		{http.MethodDelete, "/v0/projects/p1/repos/r1"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
