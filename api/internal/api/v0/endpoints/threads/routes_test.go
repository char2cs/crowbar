package threads_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/threads"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubStore struct{}

func (stubStore) Open(
	_ context.Context,
	_ reviewthread.OpenInput,
	_ time.Time,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (stubStore) Reply(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (stubStore) Resolve(
	_ context.Context,
	_ string,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (stubStore) Reopen(
	_ context.Context,
	_ string,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (stubStore) Get(
	_ context.Context,
	_ string,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (stubStore) ListByWorkspace(
	_ context.Context,
	_ string,
) ([]domain.ReviewThread, error) {
	return nil, nil
}

type stubBroadcaster struct{}

func (stubBroadcaster) Push(
	_ dto.ThreadDTO,
) {
}

// passthroughDispatch mirrors ws.DualServe's signature for the route-mount test:
// it returns the REST handler so a plain GET is served (the WS branch is
// exercised by the container integration tests).
func passthroughDispatch(
	rest gin.HandlerFunc,
	_ gin.HandlerFunc,
) gin.HandlerFunc {
	return rest
}

func noopWSHandle(
	_ *gin.Context,
) {
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	threads.Register(
		repoScoped,
		stubStore{},
		stubBroadcaster{},
		noopWSHandle,
		passthroughDispatch,
	)

	const base = "/v0/projects/p1/repos/r1/workspaces/w1/threads"
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, base},
		{http.MethodPost, base},
		{http.MethodGet, base + "/t1"},
		{http.MethodPatch, base + "/t1"},
		{http.MethodPost, base + "/t1/replies"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
