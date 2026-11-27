package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

type recordingLSP struct {
	fakeLSP
	seenOwner string
}

func (f *recordingLSP) Completion(
	_ context.Context,
	ownerID string,
	_ string,
	_ string,
	_ domlsp.Position,
) (json.RawMessage, error) {
	f.seenOwner = ownerID
	return f.completion, f.err
}

func (f *recordingLSP) DidOpen(
	_ context.Context,
	ownerID string,
	_ string,
	_ string,
	_ string,
	_ string,
) error {
	f.seenOwner = ownerID
	return f.err
}

func (f *recordingLSP) DiagnosticsSnapshot(
	ownerID string,
) []domlsp.Diagnostic {
	f.seenOwner = ownerID
	return f.snapshot
}

var _ handlers.LSPEngine = (*recordingLSP)(nil)

type recordingGit struct {
	fakeGit
	seenPath string
}

func (f *recordingGit) Blame(
	_ context.Context,
	repoPath string,
	filePath string,
) ([]gitdomain.BlameEntry, error) {
	f.seenPath = repoPath
	return f.fakeGit.Blame(context.Background(), repoPath, filePath)
}

var _ handlers.GitEngine = (*recordingGit)(nil)

func resolvedWorkspace() domain.Workspace {
	return domain.Workspace{ID: "ws-resolved", WorktreePath: "/resolved/path", Provisioning: domain.WorkspaceProvisioned}
}

func editorRouterForScopes(
	t *testing.T,
	lsp handlers.LSPEngine,
	git handlers.GitEngine,
) *gin.Engine {
	t.Helper()
	h := handlers.New(lsp, git)
	r := gin.New()

	chatScoped := r.Group("/v0/chats/:chatId")
	chatScoped.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, resolvedWorkspace())
		c.Next()
	})
	chatScoped.GET("/blame", h.Blame)
	chatScoped.POST("/lsp/completion", h.Completion)
	chatScoped.POST("/lsp/didOpen", h.DidOpen)
	chatScoped.GET("/lsp/diagnostics", h.Diagnostics)

	return r
}

func doEditorRequest(
	t *testing.T,
	r *gin.Engine,
	method, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, http.NoBody)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(rec, req)
	return rec
}

func TestChatScopedBlame_ActsOnTheResolvedWorktree(t *testing.T) {
	git := &recordingGit{}
	r := editorRouterForScopes(t, &recordingLSP{}, git)

	rec := doEditorRequest(t, r, http.MethodGet, "/v0/chats/chat-1/blame?path=main.go", "")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/resolved/path", git.seenPath)
}

func TestChatScopedLSP_KeysTheSessionByTheChatIDNotTheWorkspace(t *testing.T) {
	lsp := &recordingLSP{}
	r := editorRouterForScopes(t, lsp, &recordingGit{})

	rec := doEditorRequest(
		t, r,
		http.MethodPost, "/v0/chats/chat-1/lsp/completion",
		`{"path":"main.go","position":{"line":0,"character":0}}`,
	)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "chat-1", lsp.seenOwner)
	assert.NotEqual(t, "ws-resolved", lsp.seenOwner, "the workspace id must not key an owned session")
}

func TestChatScopedDidOpen_KeysTheSessionByTheChatID(t *testing.T) {
	lsp := &recordingLSP{}
	r := editorRouterForScopes(t, lsp, &recordingGit{})

	rec := doEditorRequest(
		t, r,
		http.MethodPost, "/v0/chats/chat-1/lsp/didOpen",
		`{"path":"main.go","languageId":"go","text":"package main"}`,
	)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "chat-1", lsp.seenOwner)
}

func TestChatScopedDiagnostics_KeysTheSnapshotByTheChatID(t *testing.T) {
	lsp := &recordingLSP{}
	r := editorRouterForScopes(t, lsp, &recordingGit{})

	rec := doEditorRequest(t, r, http.MethodGet, "/v0/chats/chat-1/lsp/diagnostics", "")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "chat-1", lsp.seenOwner)
}

func TestWorkspaceScopedRouteIsGone(t *testing.T) {
	r := editorRouterForScopes(t, &recordingLSP{}, &recordingGit{})

	rec := doEditorRequest(t, r, http.MethodGet, "/v0/workspaces/ws-direct/blame?path=main.go", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
