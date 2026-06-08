package agentrun_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agentrun"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubRepo struct{}

func (stubRepo) Create(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) (domain.AgentRun, error) {
	return domain.AgentRun{}, nil
}

func (stubRepo) ListRunning(
	_ context.Context,
) ([]domain.AgentRun, error) {
	return nil, nil
}

func (stubRepo) MarkRunning(
	_ context.Context,
	_ string,
) (domain.AgentRun, error) {
	return domain.AgentRun{}, nil
}

func (stubRepo) Complete(
	_ context.Context,
	_ string,
) (domain.AgentRun, error) {
	return domain.AgentRun{}, nil
}

func (stubRepo) Fail(
	_ context.Context,
	_ string,
) (domain.AgentRun, error) {
	return domain.AgentRun{}, nil
}

func (stubRepo) Get(
	_ context.Context,
	_ string,
) (domain.AgentRun, error) {
	return domain.AgentRun{}, nil
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	agentrun.Register(r.Group("/v0"), stubRepo{})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v0/workspaces/ws1/runs"},
		{http.MethodGet, "/v0/runs/running"},
		{http.MethodPost, "/v0/runs/r1/start"},
		{http.MethodPost, "/v0/runs/r1/complete"},
		{http.MethodPost, "/v0/runs/r1/fail"},
		{http.MethodGet, "/v0/runs/r1"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
