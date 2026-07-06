package agent_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agent"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubUsecase struct{}

func (stubUsecase) SpawnChat(
	_ context.Context,
	_ string,
	_ string,
) (string, string, error) {
	return "chat-1", "seg-1", nil
}

func (stubUsecase) IngestHook(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ []byte,
) error {
	return nil
}

func (stubUsecase) ListChats(
	_ context.Context,
) ([]domain.AgentChat, error) {
	return nil, nil
}

func (stubUsecase) GetChat(
	_ context.Context,
	id string,
) (domain.AgentChat, error) {
	return domain.AgentChat{ID: id}, nil
}

func (stubUsecase) SegmentsFor(
	_ context.Context,
	_ string,
) ([]domain.AgentSegment, error) {
	return nil, nil
}

func (stubUsecase) SwitchProvider(
	_ context.Context,
	_ string,
	_ string,
) (string, error) {
	return "seg-2", nil
}

func (stubUsecase) AssembleHandoff(
	_ context.Context,
	_ string,
) (string, error) {
	return "", nil
}

// TestRegisterMountsRoutes proves Register mounts every /v0/agent route,
// including the WS upgrade route delegating to the supplied handler.
func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	wsHit := false
	agent.Register(r.Group("/v0"), stubUsecase{}, func(c *gin.Context) {
		wsHit = true
		c.Status(http.StatusOK)
	})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v0/agent/chats"},
		{http.MethodGet, "/v0/agent/chats"},
		{http.MethodGet, "/v0/agent/chats/c1"},
		{http.MethodPost, "/v0/agent/chats/c1/switch"},
		{http.MethodGet, "/v0/agent/chats/c1/handoff"},
		{http.MethodPost, "/v0/agent/hooks"},
		{http.MethodGet, "/v0/agent/ws/chats"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		if tc.method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
		}
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
	assert.True(t, wsHit, "GET /v0/agent/ws/chats must delegate to the supplied handler")
}
