package agent_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agent"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
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

func (stubUsecase) ListChatsByWorkspace(
	_ context.Context,
	_ string,
) ([]domain.AgentChat, error) {
	return nil, nil
}

// GetChat always reports WorkspaceID "w1", matching the :wsId path segment
// TestRegisterMountsRoutes dials against, so the scope check every
// Get/Switch/Rename/Handoff route now runs (requireChatInWorkspace) passes.
func (stubUsecase) GetChat(
	_ context.Context,
	id string,
) (domain.AgentChat, error) {
	return domain.AgentChat{ID: id, WorkspaceID: "w1"}, nil
}

// LiveRunnerForChat answers agentrunner.ErrNotFound — "this chat is DORMANT", the
// honest answer for a stub that starts no process, and not a failure: a live-runner row
// exists exactly while a PTY does, so its absence IS the liveness verdict. The read
// routes therefore still serve 200 with empty liveRunnerId/terminalSessionId, which is
// what the mount test asserts on.
func (stubUsecase) LiveRunnerForChat(
	_ context.Context,
	_ string,
) (domain.AgentRunner, error) {
	return domain.AgentRunner{}, agentrunner.ErrNotFound
}

func (stubUsecase) ConversationsForChat(
	_ context.Context,
	_ string,
) ([]domain.ChatConversation, error) {
	return nil, nil
}

func (stubUsecase) SwitchProvider(
	_ context.Context,
	_ string,
	_ string,
) (string, error) {
	return "seg-2", nil
}

func (stubUsecase) ResumeChat(
	_ context.Context,
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

func (stubUsecase) RenameChat(
	_ context.Context,
	_, _, _ string,
) error {
	return nil
}

func (stubUsecase) RenameByRunner(
	_ context.Context,
	_, _, _ string,
) error {
	return nil
}

func (stubUsecase) PurgeChat(
	_ context.Context,
	_ string,
) error {
	return nil
}

func (stubUsecase) ListProviders(
	_ context.Context,
	_ string,
) ([]engineagent.Descriptor, error) {
	return nil, nil
}

// TestRegisterMountsRoutes proves Register mounts every agent route nested
// under the workspace-scoped group (Task 3: .../workspaces/:wsId/agent/...),
// including the WS upgrade route delegating to the supplied handler.
func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	wsScoped := r.Group("/v0/projects/:projectId/repos/:repoId/workspaces/:wsId")
	wsHit := false
	agent.Register(wsScoped, stubUsecase{}, func(c *gin.Context) {
		wsHit = true
		c.Status(http.StatusOK)
	})

	const base = "/v0/projects/p1/repos/r1/workspaces/w1"
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, base + "/agent/chats"},
		{http.MethodGet, base + "/agent/chats"},
		{http.MethodGet, base + "/agent/chats/c1"},
		{http.MethodPost, base + "/agent/chats/c1/switch"},
		{http.MethodPost, base + "/agent/chats/c1/rename"},
		{http.MethodGet, base + "/agent/chats/c1/handoff"},
		{http.MethodDelete, base + "/agent/chats/c1"},
		{http.MethodPost, base + "/agent/runners/seg-1/rename"},
		{http.MethodPost, base + "/agent/hooks"},
		{http.MethodGet, base + "/agent/providers"},
		{http.MethodGet, base + "/agent/ws/chats"},
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
	assert.True(t, wsHit, "GET .../agent/ws/chats must delegate to the supplied handler")
}
