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

// This file pins the handler half of editor/LSP's move onto /v0/chats/:chatId
// (spec §4.2's OWNED bucket, §8 step 5): WHICH worktree a handler resolves for
// each of its two live mounts, and WHICH key the LSP engine's per-session pool
// is addressed by on each.
//
// Editor/LSP diverges from git/files' shared-bucket shape here: git blame (and
// the worktree lookup itself) resolves the same worktree either way, but the
// LSP engine calls must NOT key a chat-scoped session by the resolved
// workspace id — that would let a sibling chat sharing this worktree collide
// with, or read, this chat's own LSP session, exactly the sharing spec law 5
// exists to rule out.

// recordingLSP embeds fakeLSP and records the owner id each call was made
// with, so a test can assert the CHAT id reached the engine on the new mount
// and the WORKSPACE id still does on the old one.
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

// recordingGit embeds fakeGit and records the worktree path Blame was called
// with.
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

// resolvedWorkspace is the workspace the chat group's resolveChatWorktree
// middleware stashes for chat "chat-1". Its id and worktree path deliberately
// differ from every chat id / old-mount fixture below, so a handler that
// reached for the wrong source would fail these assertions rather than pass
// them by coincidence.
func resolvedWorkspace() domain.Workspace {
	return domain.Workspace{ID: "ws-resolved", WorktreePath: "/resolved/path"}
}

// editorRouterForScopes wires editor's two live mounts the way router.go
// does, including the chat group's middleware — the piece that makes a
// chat-scoped request resolvable at all.
func editorRouterForScopes(
	t *testing.T,
	lsp handlers.LSPEngine,
	git handlers.GitEngine,
) *gin.Engine {
	t.Helper()
	h := handlers.New(lsp, git, okWSReader())
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

	wsScoped := r.Group("/v0/workspaces/:wsId")
	wsScoped.GET("/blame", h.Blame)
	wsScoped.POST("/lsp/completion", h.Completion)
	wsScoped.POST("/lsp/didOpen", h.DidOpen)
	wsScoped.GET("/lsp/diagnostics", h.Diagnostics)

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

// TestChatScopedBlame_ActsOnTheResolvedWorktree proves a git blame reached
// through a chat operates on the WORKSPACE that chat resolves to — the same
// shared-bucket-shaped resolution git/files use, since git blame has no
// session of its own to own.
func TestChatScopedBlame_ActsOnTheResolvedWorktree(t *testing.T) {
	git := &recordingGit{}
	r := editorRouterForScopes(t, &recordingLSP{}, git)

	rec := doEditorRequest(t, r, http.MethodGet, "/v0/chats/chat-1/blame?path=main.go", "")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/resolved/path", git.seenPath)
}

// TestChatScopedLSP_KeysTheSessionByTheChatIDNotTheWorkspace is the core
// assertion this step exists for: a chat-scoped LSP call keys the engine's
// session pool by the CHAT id, not the workspace it resolves to — so a
// sibling chat sharing this worktree gets a different key and therefore its
// own session (spec law 5).
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

// TestChatScopedDidOpen_KeysTheSessionByTheChatID covers the document-sync
// notification path, which reads the same owner-id seam through a different
// handler and so could have been missed by a partial re-key.
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

// TestChatScopedDiagnostics_KeysTheSnapshotByTheChatID covers the diagnostics
// snapshot read, the third handler that keys an LSP engine call by owner id.
func TestChatScopedDiagnostics_KeysTheSnapshotByTheChatID(t *testing.T) {
	lsp := &recordingLSP{}
	r := editorRouterForScopes(t, lsp, &recordingGit{})

	rec := doEditorRequest(t, r, http.MethodGet, "/v0/chats/chat-1/lsp/diagnostics", "")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "chat-1", lsp.seenOwner)
}

// TestWorkspaceScopedRoutesStillActOnTheirPathParam is the regression bar for
// the mount this step deliberately leaves standing: the old route keeps
// naming its workspace directly, and reqscope — never set on that group —
// must not have displaced it, for either the shared blame read or the owned
// LSP session key.
func TestWorkspaceScopedRoutesStillActOnTheirPathParam(t *testing.T) {
	lsp := &recordingLSP{}
	git := &recordingGit{}
	r := editorRouterForScopes(t, lsp, git)

	rec := doEditorRequest(t, r, http.MethodGet, "/v0/workspaces/ws-direct/blame?path=main.go", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/repo", git.seenPath, "the old mount resolves via wsReader, not reqscope")

	rec = doEditorRequest(
		t, r,
		http.MethodPost, "/v0/workspaces/ws-direct/lsp/completion",
		`{"path":"main.go","position":{"line":0,"character":0}}`,
	)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ws-direct", lsp.seenOwner, "the old mount keys the LSP session by :wsId, unchanged")
}
