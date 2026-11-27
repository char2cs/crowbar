package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type recordingFiles struct {
	stubFiles
	seen []string
}

func (r *recordingFiles) record(
	wsID string,
) {
	r.seen = append(r.seen, wsID)
}

func (r *recordingFiles) last(
	t *testing.T,
) string {
	t.Helper()
	require.NotEmpty(t, r.seen, "the handler never reached the file usecase at all")
	return r.seen[len(r.seen)-1]
}

func (r *recordingFiles) Tree(
	_ context.Context,
	wsID string,
	_ string,
	_ file.FileStatusProvider,
) ([]domain.FileNode, error) {
	r.record(wsID)
	return []domain.FileNode{}, nil
}

func (r *recordingFiles) CreateFile(
	_ context.Context,
	wsID string,
	_ string,
	_ time.Time,
) error {
	r.record(wsID)
	return nil
}

func (r *recordingFiles) Delete(
	_ context.Context,
	wsID string,
	_ string,
	_ time.Time,
) error {
	r.record(wsID)
	return nil
}

func resolvedWorkspace() domain.Workspace {
	return domain.Workspace{ID: "ws-resolved", ProjectID: "p1", RepoID: "r1"}
}

func filesRouterForScopes(
	t *testing.T,
	f handlers.Files,
) *gin.Engine {
	t.Helper()
	h := handlers.New(f)
	r := gin.New()

	chatScoped := r.Group("/v0/chats/:chatId")
	chatScoped.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, resolvedWorkspace())
		c.Next()
	})
	chatScoped.GET("/files/tree", h.Tree)
	chatScoped.POST("/files", h.Create)
	chatScoped.DELETE("/files", h.Delete)

	wsScoped := r.Group("/v0")
	wsScoped.GET("/workspaces/:wsId/files/tree", h.Tree)
	wsScoped.POST("/workspaces/:wsId/files", h.Create)
	wsScoped.DELETE("/workspaces/:wsId/files", h.Delete)

	return r
}

func TestChatScopedRead_ActsOnTheResolvedWorktreeNotTheChat(t *testing.T) {
	files := &recordingFiles{}
	r := filesRouterForScopes(t, files)

	rec := do(r, http.MethodGet, "/v0/chats/chat-1/files/tree", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	got := files.last(t)
	assert.Equal(t, "ws-resolved", got)
	assert.NotEqual(t, "chat-1", got, "the chat id is not a workspace id")
	assert.NotEmpty(t, got, "an unresolved :wsId would silently aim the file usecase at nothing")
}

func TestChatScopedWrite_ActsOnTheResolvedWorktree(t *testing.T) {
	files := &recordingFiles{}
	r := filesRouterForScopes(t, files)

	rec := do(r, http.MethodPost, "/v0/chats/chat-1/files", map[string]string{"path": "a.go"})

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "ws-resolved", files.last(t))
}

func TestChatScopedDelete_ActsOnTheResolvedWorktree(t *testing.T) {
	files := &recordingFiles{}
	r := filesRouterForScopes(t, files)

	rec := do(r, http.MethodDelete, "/v0/chats/chat-1/files", map[string]string{"path": "a.go"})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ws-resolved", files.last(t))
}

func TestWorkspaceScopedRoutesStillActOnTheirPathParam(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		body   any
		status int
	}{
		{"read", http.MethodGet, "/v0/workspaces/ws-direct/files/tree", nil, http.StatusOK},
		{"write", http.MethodPost, "/v0/workspaces/ws-direct/files", map[string]string{"path": "a.go"}, http.StatusCreated},
		{"delete", http.MethodDelete, "/v0/workspaces/ws-direct/files", map[string]string{"path": "a.go"}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := &recordingFiles{}
			r := filesRouterForScopes(t, files)

			rec := do(r, tc.method, tc.path, tc.body)

			require.Equal(t, tc.status, rec.Code)
			assert.Equal(t, "ws-direct", files.last(t))
		})
	}
}

func TestBothMountsReachTheSameHandlerSet(t *testing.T) {
	files := &recordingFiles{}
	r := filesRouterForScopes(t, files)

	require.Equal(t, http.StatusOK,
		do(r, http.MethodGet, "/v0/chats/chat-1/files/tree", nil).Code)
	viaChat := files.last(t)

	require.Equal(t, http.StatusOK,
		do(r, http.MethodGet, "/v0/workspaces/ws-resolved/files/tree", nil).Code)
	viaWorkspace := files.last(t)

	assert.Equal(t, viaWorkspace, viaChat,
		"one worktree named two ways must reach the file usecase identically")
}
