package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chats/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// configurableUsecase lets each test dial in the ForkChat/DeleteChat behavior
// needed to exercise their error branches.
type configurableUsecase struct {
	forkChat domain.Chat
	forkErr  error
	delErr   error
}

func (configurableUsecase) CreateChat(_ context.Context, id, wsID, title string, _ time.Time) (domain.Chat, error) {
	return domain.Chat{ID: id, WsID: wsID, Title: title}, nil
}

func (c configurableUsecase) ForkChat(_ context.Context, parentID string, _ time.Time) (domain.Chat, error) {
	if c.forkErr != nil {
		return domain.Chat{}, c.forkErr
	}
	return c.forkChat, nil
}

func (configurableUsecase) RenameChat(_ context.Context, id, title string) (domain.Chat, error) {
	return domain.Chat{ID: id, Title: title}, nil
}

func (c configurableUsecase) DeleteChat(_ context.Context, _ string, _ time.Time) error {
	return c.delErr
}

// configurableRepo lets each test dial in the ListByWorkspace behavior needed
// to exercise List's empty/non-empty/error branches.
type configurableRepo struct {
	chats   []domain.Chat
	listErr error
}

func (c configurableRepo) ListByWorkspace(_ context.Context, wsID string) ([]domain.Chat, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return c.chats, nil
}

// configurableWsReader lets each test dial in the workspace Get behavior
// needed to exercise List's 404-on-unknown-workspace branch.
type configurableWsReader struct {
	getErr error
}

func (c configurableWsReader) Get(_ context.Context, id string) (domain.Workspace, error) {
	if c.getErr != nil {
		return domain.Workspace{}, c.getErr
	}
	return domain.Workspace{ID: id}, nil
}

// newRouterWithWs mirrors newRouter but lets tests substitute a workspace
// reader that can fail, to exercise List's 404-on-unknown-workspace branch.
func newRouterWithWs(
	uc handlers.ChatUsecase,
	repo handlers.ChatRepo,
	wsReader handlers.WorkspaceReader,
) *gin.Engine {
	r := gin.New()
	h := handlers.New(uc, repo, wsReader)
	rg := r.Group("/v0")
	rg.POST("/workspaces/:wsId/chats", h.Create)
	rg.GET("/workspaces/:wsId/chats", h.List)
	rg.POST("/chats/:id/fork", h.Fork)
	rg.PATCH("/chats/:id", h.Rename)
	rg.DELETE("/chats/:id", h.Delete)
	return r
}

// TestList_WorkspaceNotFound pins that List 404s when the workspace itself
// does not exist, instead of serving an empty chat list.
func TestList_WorkspaceNotFound(
	t *testing.T,
) {
	r := newRouterWithWs(stubUsecase{}, configurableRepo{}, configurableWsReader{getErr: errors.New("no such workspace")})

	rec := do(r, http.MethodGet, "/v0/workspaces/missing/chats", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestList_Empty pins that a workspace with no chats yet returns 200 with an
// empty list.
func TestList_Empty(
	t *testing.T,
) {
	r := newRouterWithWs(stubUsecase{}, configurableRepo{chats: []domain.Chat{}}, configurableWsReader{})

	rec := do(r, http.MethodGet, "/v0/workspaces/ws1/chats", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []domain.Chat `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Empty(t, body.Data)
}

// TestList_NonEmpty pins that a workspace with chats returns them all.
func TestList_NonEmpty(
	t *testing.T,
) {
	repo := configurableRepo{chats: []domain.Chat{{ID: "c1"}, {ID: "c2"}}}
	r := newRouterWithWs(stubUsecase{}, repo, configurableWsReader{})

	rec := do(r, http.MethodGet, "/v0/workspaces/ws1/chats", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []domain.Chat `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 2)
}

// TestList_RepoError pins that a chat repo failure surfaces as a 500.
func TestList_RepoError(
	t *testing.T,
) {
	r := newRouterWithWs(stubUsecase{}, configurableRepo{listErr: errors.New("db down")}, configurableWsReader{})

	rec := do(r, http.MethodGet, "/v0/workspaces/ws1/chats", nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestFork_Success pins that forking an existing chat returns 201 with the
// forked chat.
func TestFork_Success(
	t *testing.T,
) {
	uc := configurableUsecase{forkChat: domain.Chat{ID: "fork-c1", ParentID: "c1"}}
	r := newRouterWithWs(uc, configurableRepo{}, configurableWsReader{})

	rec := do(r, http.MethodPost, "/v0/chats/c1/fork", nil)
	assert.Equal(t, http.StatusCreated, rec.Code)
	var body struct {
		Data domain.Chat `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "fork-c1", body.Data.ID)
}

// TestFork_NotFound pins that forking a non-existent chat surfaces the
// usecase error as a 500 (the handler has no dedicated not-found mapping).
func TestFork_NotFound(
	t *testing.T,
) {
	uc := configurableUsecase{forkErr: errors.New("chat not found")}
	r := newRouterWithWs(uc, configurableRepo{}, configurableWsReader{})

	rec := do(r, http.MethodPost, "/v0/chats/missing/fork", nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestDelete_Success pins that deleting an existing chat returns 204.
func TestDelete_Success(
	t *testing.T,
) {
	r := newRouterWithWs(configurableUsecase{}, configurableRepo{}, configurableWsReader{})

	rec := do(r, http.MethodDelete, "/v0/chats/c1", nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// TestDelete_NotFound pins that deleting a non-existent chat surfaces the
// usecase error as a 500 (the handler has no dedicated not-found mapping).
func TestDelete_NotFound(
	t *testing.T,
) {
	uc := configurableUsecase{delErr: errors.New("chat not found")}
	r := newRouterWithWs(uc, configurableRepo{}, configurableWsReader{})

	rec := do(r, http.MethodDelete, "/v0/chats/missing", nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
