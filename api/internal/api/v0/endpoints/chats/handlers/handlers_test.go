package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chats/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubUsecase struct{}

func (stubUsecase) CreateChat(_ context.Context, id, wsID, title string, _ time.Time) (domain.Chat, error) {
	return domain.Chat{ID: id, WsID: wsID, Title: title}, nil
}
func (stubUsecase) ForkChat(_ context.Context, parentID string, _ time.Time) (domain.Chat, error) {
	return domain.Chat{ID: "fork-" + parentID}, nil
}
func (stubUsecase) RenameChat(_ context.Context, id, title string) (domain.Chat, error) {
	return domain.Chat{ID: id, Title: title}, nil
}
func (stubUsecase) DeleteChat(_ context.Context, _ string, _ time.Time) error { return nil }

type stubRepo struct{}

func (stubRepo) ListByWorkspace(_ context.Context, wsID string) ([]domain.Chat, error) {
	return []domain.Chat{{ID: "c1", WsID: wsID}}, nil
}

type stubWsReader struct{}

func (stubWsReader) Get(_ context.Context, id string) (domain.Workspace, error) {
	return domain.Workspace{ID: id}, nil
}

func newRouter(
	uc handlers.ChatUsecase,
	repo handlers.ChatRepo,
) *gin.Engine {
	r := gin.New()
	h := handlers.New(uc, repo, stubWsReader{})
	rg := r.Group("/v0")
	rg.POST("/workspaces/:wsId/chats", h.Create)
	rg.GET("/workspaces/:wsId/chats", h.List)
	rg.POST("/chats/:id/fork", h.Fork)
	rg.PATCH("/chats/:id", h.Rename)
	rg.DELETE("/chats/:id", h.Delete)
	return r
}

func do(
	r *gin.Engine,
	method, path string,
	body any,
) *httptest.ResponseRecorder {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestChatHandlers_HappyPath(
	t *testing.T,
) {
	r := newRouter(stubUsecase{}, stubRepo{})

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/chats", map[string]any{"title": "chat"})
	assert.Equal(t, http.StatusCreated, rec.Code)

	rec = do(r, http.MethodGet, "/v0/workspaces/ws1/chats", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodPost, "/v0/chats/c1/fork", nil)
	assert.Equal(t, http.StatusCreated, rec.Code)

	rec = do(r, http.MethodPatch, "/v0/chats/c1", map[string]any{"title": "new"})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodDelete, "/v0/chats/c1", nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}
