package projects_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
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

func (stubReader) Update(
	_ context.Context,
	_ string,
	_ project.Update,
) (domain.Project, error) {
	return domain.Project{}, nil
}

func (stubReader) Reorder(
	_ context.Context,
	_ string,
	_ int,
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

type stubOps struct{}

func (stubOps) Start(context.Context, operation.Spec, operation.Work) (domain.Operation, error) {
	return domain.Operation{}, nil
}

func (stubOps) Replay(context.Context, string, domain.OperationKind, domain.EntityRef) (domain.Operation, bool, error) {
	return domain.Operation{}, false, nil
}

func (stubImporter) Create(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (domain.Project, error) {
	return domain.Project{}, nil
}

type stubDeleter struct{}

func (stubDeleter) BeginDelete(
	_ context.Context,
	id string,
	_ domain.DeleteConsent,
) (domain.Project, error) {
	return domain.Project{ID: id}, nil
}

func (stubDeleter) Delete(
	_ context.Context,
	_ string,
	_ domain.DeleteConsent,
) error {
	return nil
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	projects.Register(
		r.Group("/v0"),
		stubReader{},
		stubImporter{},
		stubDeleter{},
		stubOps{},
		func() (string, error) { return t.TempDir(), nil },
	)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/projects"},
		{http.MethodPatch, "/v0/projects/p1"},
		{http.MethodPost, "/v0/projects"},
		{http.MethodGet, "/v0/projects/abc"},
		{http.MethodDelete, "/v0/projects/abc"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
