//go:build integration

package v0_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/testutil/accessfake"
)

type workspaceGetter interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}

type stubChatWorktreeResolver struct {
	chatToWs   map[string]string
	workspaces workspaceGetter
}

func (s stubChatWorktreeResolver) Resolve(
	ctx context.Context,
	chatID string,
) (domain.Workspace, error) {
	wsID, ok := s.chatToWs[chatID]
	if !ok {
		return domain.Workspace{}, problem.ErrNotFound
	}
	return s.workspaces.Get(ctx, wsID)
}

func expectedRoutes() map[string]struct{} {
	out := map[string]struct{}{}
	for _, r := range append(append([]string{}, specRoutes()...), extraRoutes()...) {
		out[r] = struct{}{}
	}
	return out
}

func specRoutes() []string {
	const repo = "/v0/projects/:projectId/repos/:repoId"
	const ws = repo + "/workspaces/:wsId"
	const chat = "/v0/chats/:chatId"
	return []string{
		"GET /v0/projects",
		"POST /v0/projects",
		"GET /v0/projects/:projectId",
		"GET /v0/projects/:projectId/repos",
		"POST /v0/projects/:projectId/repos",
		"GET " + repo,
		"GET " + chat + "/files/tree",
		"POST " + chat + "/files",
		"PATCH " + chat + "/files",
		"DELETE " + chat + "/files",
		"GET " + chat + "/blame",
		"POST " + chat + "/lsp/completion",
		"POST " + chat + "/lsp/hover",
		"POST " + chat + "/lsp/definition",
		"POST " + chat + "/lsp/references",
		"POST " + chat + "/lsp/rename",
		"POST " + chat + "/lsp/codeAction",
		"POST " + chat + "/lsp/documentSymbol",
		"POST " + chat + "/lsp/signatureHelp",
		"POST " + chat + "/lsp/codeLens",
		"POST " + chat + "/lsp/codeLensResolve",
		"POST " + chat + "/lsp/semanticTokens",
		"POST " + chat + "/lsp/semanticTokensRange",
		"POST " + chat + "/lsp/executeCommand",
		"POST " + chat + "/lsp/formatting",
		"GET " + chat + "/lsp/status",
		"POST " + chat + "/lsp/restart",
		"GET " + chat + "/lsp/diagnostics",
		"GET " + chat + "/git/status",
		"GET " + chat + "/git/log",
		"GET " + chat + "/git/diff",
		"GET " + chat + "/git/branches",
		"GET " + chat + "/git/stashes",
		"GET " + chat + "/git/conflicts",
		"GET " + chat + "/git/conflict-hunks",
		"GET " + chat + "/git/commit-diff",
		"POST " + chat + "/git/stage",
		"POST " + chat + "/git/stage-hunk",
		"POST " + chat + "/git/unstage",
		"POST " + chat + "/git/unstage-hunk",
		"POST " + chat + "/git/discard",
		"POST " + chat + "/git/commit",
		"POST " + chat + "/git/push",
		"POST " + chat + "/git/pull",
		"POST " + chat + "/git/fetch",
		"POST " + chat + "/git/branches",
		"PATCH " + chat + "/git/branches",
		"DELETE " + chat + "/git/branches",
		"POST " + chat + "/git/switch",
		"POST " + chat + "/git/stash",
		"POST " + chat + "/git/stash-apply",
		"POST " + chat + "/git/stash-pop",
		"DELETE " + chat + "/git/stash",
		"POST " + chat + "/git/reset",
		"POST " + chat + "/git/merge",
		"POST " + chat + "/git/rebase",
		"POST " + chat + "/git/resolve-hunk",
		"POST " + chat + "/git/operation/continue",
		"POST " + chat + "/git/operation/abort",
		"GET " + chat + "/review",
		"PATCH " + chat + "/review",
		"GET " + ws + "/threads",
		"POST " + ws + "/threads",
		"GET " + ws + "/threads/:threadId",
		"PATCH " + ws + "/threads/:threadId",
		"POST " + ws + "/threads/:threadId/replies",
		"GET " + chat + "/provider",
		"GET " + repo + "/protected-branches",
		"GET " + chat + "/blobs/:ref",
		"GET " + chat + "/rows",
		"GET " + chat + "/rows/:seq",
		"POST " + chat + "/search",
		"POST " + chat + "/search/replace",
		"GET /v0/settings/terminal/profiles",
		"GET /v0/settings/terminal/profiles/:id",
		"POST /v0/settings/terminal/profiles",
		"PUT /v0/settings/terminal/profiles/:id",
		"DELETE /v0/settings/terminal/profiles/:id",
		"GET " + chat + "/terminals",
		"POST " + chat + "/terminals",
		"DELETE " + chat + "/terminals/:sessionId",
		"GET /v0/system/prerequisites/:tool",
		"GET /v0/system/browse",
		"GET /v0/health",
		"GET " + chat + "/terminals/:sessionId/ws",
	}
}

func extraRoutes() []string {
	const repo = "/v0/projects/:projectId/repos/:repoId"
	const ws = repo + "/workspaces/:wsId"
	const home = "/v0/projects/:projectId/home"
	const chat = "/v0/chats/:chatId"
	return []string{
		"POST /v0/relays/:relay/mcp",
		"DELETE /v0/projects/:projectId",
		"PATCH /v0/projects/:projectId",
		"DELETE " + repo,
		"PATCH " + repo,
		"GET " + repo + "/pull-requests",
		"GET " + repo + "/icon",
		"PUT " + repo + "/icon",
		"DELETE " + repo + "/icon",
		"PUT " + repo + "/icon/emoji",
		"PUT " + repo + "/icon/github",
		"GET /v0/projects/:projectId/icon",
		"PUT /v0/projects/:projectId/icon",
		"DELETE /v0/projects/:projectId/icon",
		"PUT /v0/projects/:projectId/icon/emoji",
		"GET " + repo + "/branches",
		"GET " + repo + "/workspaces",
		"GET " + repo + "/pull-requests",
		"POST " + chat + "/files/copy",
		"GET " + chat + "/review/files",
		"GET " + chat + "/review/outline",
		"GET " + chat + "/review/patch",
		"GET " + chat + "/review/search",
		"GET " + chat + "/review/contents",
		"POST " + chat + "/lsp/didOpen",
		"POST " + chat + "/lsp/didChange",
		"POST " + chat + "/lsp/didClose",
		"POST " + chat + "/lsp/didSave",
		"GET " + chat + "/identity",
		"DELETE " + ws + "/threads/:threadId",
		"PATCH " + ws + "/threads/:threadId/messages/:messageId",
		"DELETE " + ws + "/threads/:threadId/messages/:messageId",
		"PATCH " + ws + "/placement",
		"POST " + repo + "/chats",
		"GET " + repo + "/chats",
		"GET " + repo + "/chats/:id",
		"POST " + repo + "/chats/:id/switch",
		"POST " + repo + "/chats/:id/rename",
		"GET " + repo + "/chats/:id/handoff",
		"DELETE " + repo + "/chats/:id",
		"PATCH " + repo + "/chats/:id/placement",
		"POST " + repo + "/chats/:id/branch-out",
		"POST " + repo + "/chats/:id/move/cancel",
		"POST " + repo + "/chats/:id/lock",
		"POST " + repo + "/chats/:id/sync",
		"POST " + repo + "/chats/:id/merge-into-parent",
		"POST " + repo + "/chats/:id/reparent",
		"POST " + repo + "/chats/:id/rebase-onto-parent",
		"POST " + repo + "/chats/:id/retry-provision",
		"POST " + repo + "/chats/:id/detach-holder",
		"PATCH " + repo + "/chats/:id/branch",
		"POST " + repo + "/chats/import-batch",
		"GET " + repo + "/chats/folders",
		"POST " + repo + "/chats/folders",
		"PATCH " + repo + "/chats/folders/:folderId",
		"DELETE " + repo + "/chats/folders/:folderId",
		"GET " + repo + "/chats/providers",
		"POST " + repo + "/chats/:id/resume",
		"POST " + repo + "/chats/runners/:segid/mcp",
		"POST " + repo + "/chats/runners/:segid/hooks/:provider/:event",
		"POST " + repo + "/chats/:id/stop",
		"POST " + chat + "/kill",
		"POST " + repo + "/chats/:id/terminal-wait/answer",
		"POST " + repo + "/chats/:id/compact",
		"POST " + repo + "/chats/:id/shells/:shellId/stop",
		"POST " + repo + "/chats/:id/switch-to-terminal",
		"POST " + repo + "/chats/:id/switch-to-native",
		"POST " + repo + "/chats/:id/attachments",
		"GET " + repo + "/chats/:id/attachments/:file",
		"HEAD " + repo + "/chats/:id/attachments/:file",
		"POST " + repo + "/chats/:id/attachments/uploads",
		"PUT " + repo + "/chats/:id/attachments/uploads/:uploadId",
		"POST " + repo + "/chats/:id/attachments/uploads/:uploadId/complete",
		"DELETE " + repo + "/chats/:id/attachments/uploads/:uploadId",
		"GET " + repo + "/chats/:id/telemetry",
		"GET " + repo + "/chats/:id/slash-catalog",
		"PATCH " + repo + "/chats/:id/selection",
		"PUT " + repo + "/chats/:id/permission-level",
		"POST " + repo + "/chats/:id/choices/:choiceId/answer",
		"POST " + repo + "/chats/:id/prompts",
		"POST " + repo + "/chats/:id/prompts/:requestId/retract",
		"PUT /v0/settings/chat/providers",
		"GET /v0/settings/chat/providers",
		"GET /v0/settings/chat/permission-level",
		"PUT /v0/settings/chat/permission-level",
		"GET /v0/settings/chat/model-manifest-fetch",
		"PUT /v0/settings/chat/model-manifest-fetch",
		"GET /v0/settings/chat/descriptors",
		"POST /v0/settings/chat/descriptors",
		"GET /v0/settings/terminal/theme", "PUT /v0/settings/terminal/theme",
		"GET /v0/documents/content", "PUT /v0/documents/content",
		"GET " + home,
		"GET " + home + "/files/tree",
		"POST " + home + "/files",
		"PATCH " + home + "/files",
		"DELETE " + home + "/files",
		"GET " + home + "/terminals",
		"POST " + home + "/terminals",
		"DELETE " + home + "/terminals/:sessionId",
		"GET " + home + "/terminals/:sessionId/ws",
		"GET " + home + "/threads",
		"POST " + home + "/threads",
		"GET " + home + "/threads/:threadId",
		"PATCH " + home + "/threads/:threadId",
		"DELETE " + home + "/threads/:threadId",
		"POST " + home + "/threads/:threadId/replies",
		"PATCH " + home + "/threads/:threadId/messages/:messageId",
		"DELETE " + home + "/threads/:threadId/messages/:messageId",
		"POST " + home + "/chats",
		"GET " + home + "/chats",
		"GET " + home + "/chats/:id",
		"POST " + home + "/chats/:id/switch",
		"POST " + home + "/chats/:id/rename",
		"GET " + home + "/chats/:id/handoff",
		"DELETE " + home + "/chats/:id",
		"GET " + home + "/chats/providers",
		"POST " + home + "/chats/:id/switch-to-terminal",
		"POST " + home + "/chats/:id/switch-to-native",
		"POST " + home + "/chats/:id/resume",
		"POST " + home + "/chats/:id/switch-to-terminal",
		"POST " + home + "/chats/:id/switch-to-native",
		"PUT " + home + "/chats/:id/permission-level",
		"POST " + home + "/chats/runners/:segid/mcp",
		"POST " + home + "/chats/runners/:segid/hooks/:provider/:event",
		"POST " + home + "/chats/:id/stop",
		"POST " + home + "/chats/:id/terminal-wait/answer",
		"POST " + home + "/chats/:id/compact",
		"POST " + home + "/chats/:id/shells/:shellId/stop",
		"PATCH " + home + "/chats/:id/placement",
		"POST " + home + "/chats/:id/branch-out",
		"POST " + home + "/chats/:id/move/cancel",
		"GET " + home + "/chats/folders",
		"POST " + home + "/chats/folders",
		"PATCH " + home + "/chats/folders/:folderId",
		"DELETE " + home + "/chats/folders/:folderId",
		"GET " + home + "/chats/:id/telemetry",
		"POST " + home + "/chats/:id/attachments",
		"GET " + home + "/chats/:id/attachments/:file",
		"HEAD " + home + "/chats/:id/attachments/:file",
		"POST " + home + "/chats/:id/attachments/uploads",
		"PUT " + home + "/chats/:id/attachments/uploads/:uploadId",
		"POST " + home + "/chats/:id/attachments/uploads/:uploadId/complete",
		"DELETE " + home + "/chats/:id/attachments/uploads/:uploadId",
		"GET " + home + "/chats/:id/slash-catalog",
		"PATCH " + home + "/chats/:id/selection",
		"PUT " + home + "/chats/:id/permission-level",
		"POST " + home + "/chats/:id/choices/:choiceId/answer",
		"POST " + home + "/chats/:id/prompts",
		"POST " + home + "/chats/:id/prompts/:requestId/retract",
		"POST " + home + "/files/copy",
		"GET /v0/system/perf",
		"POST /v0/system/perf",
		"GET /v0/stream",
		"DELETE /v0/operations/:id",
		"POST /v0/pair/code",
		"POST /v0/pair",
		"GET /v0/access",
		"PUT /v0/access",
		"GET /v0/clients",
		"DELETE /v0/clients/:id",
		"GET /v0/browser/peer",
	}
}

func registeredRoutes(
	t *testing.T,
) map[string]struct{} {
	t.Helper()
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng, accessfake.Closed())
	r := gin.New()
	c.Register(r.Group("/v0"))
	out := map[string]struct{}{}
	for _, rt := range r.Routes() {
		out[rt.Method+" "+rt.Path] = struct{}{}
	}
	return out
}

func TestRouteAudit_AllSpecRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	got := registeredRoutes(t)
	want := expectedRoutes()

	for r := range want {
		_, ok := got[r]
		assert.Truef(t, ok, "spec route not registered: %s", r)
	}
	for r := range got {
		_, ok := want[r]
		assert.Truef(t, ok, "registered route not in spec/superset: %s", r)
	}
	assert.Len(t, got, len(want), "registered route count drifted from expected set")
}

func TestRouteAudit_NoLegacyWsRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	got := registeredRoutes(t)

	const repo = "/v0/projects/:projectId/repos/:repoId"
	legacy := []string{
		"GET " + repo + "/ws/workspaces",
		"GET " + repo + "/ws/git",
		"GET " + repo + "/ws/files",
		"GET " + repo + "/ws/lsp",
		"GET " + repo + "/ws/terminals/:sessionId",
		"GET " + repo + "/chats/ws",
		"GET /v0/chats/:chatId/ws",
		"GET /v0/projects/:projectId/home/chats/ws",
	}
	for _, r := range legacy {
		_, ok := got[r]
		assert.Falsef(t, ok, "legacy /ws/* route must be removed: %s", r)
	}
	for r := range got {
		assert.NotContainsf(t, r, "/ws/workspaces", "legacy ws route present: %s", r)
		assert.NotContainsf(t, r, "/ws/git", "legacy ws route present: %s", r)
		assert.NotContainsf(t, r, "/ws/lsp", "legacy ws route present: %s", r)
	}
}

func TestRouteAudit_NoWorkspaceGroupRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	got := registeredRoutes(t)

	const repo = "/v0/projects/:projectId/repos/:repoId"
	const ws = repo + "/workspaces/:wsId"
	deleted := []string{
		"GET " + ws,
		"POST " + repo + "/workspaces",
		"POST " + repo + "/workspaces/import",
		"PATCH " + ws,
		"DELETE " + ws,
		"POST " + ws + "/sync",
		"POST " + ws + "/lock",
		"POST " + ws + "/merge-into-parent",
		"POST " + ws + "/reparent",
		"POST " + ws + "/rebase-onto-parent",
		"POST " + ws + "/retry-provision",
		"POST " + ws + "/detach-holder",
	}
	require.Len(t, deleted, 12, "the group mounted thirteen routes; twelve stay deleted (the LIST route came back)")
	for _, r := range deleted {
		_, ok := got[r]
		assert.Falsef(t, ok, "deleted workspaces-group route is still registered: %s", r)
	}

	_, listRestored := got["GET "+repo+"/workspaces"]
	assert.True(t, listRestored, "the workspace LIST route must be registered — see this test's own doc")

	for _, r := range []string{
		"POST " + repo + "/chats",
		"GET " + repo + "/chats",
		"GET " + repo + "/chats/:id",
		"DELETE " + repo + "/chats/:id",
		"PATCH " + repo + "/chats/:id/placement",
		"PATCH " + repo + "/chats/:id/branch",
		"POST " + repo + "/chats/import-batch",
		"POST " + repo + "/chats/:id/sync",
		"POST " + repo + "/chats/:id/lock",
		"POST " + repo + "/chats/:id/merge-into-parent",
		"POST " + repo + "/chats/:id/reparent",
		"POST " + repo + "/chats/:id/rebase-onto-parent",
		"POST " + repo + "/chats/:id/retry-provision",
		"POST " + repo + "/chats/:id/detach-holder",
	} {
		_, ok := got[r]
		assert.Truef(t, ok, "chat-keyed replacement missing: %s", r)
	}

	const home = "/v0/projects/:projectId/home"
	for _, r := range []string{
		"GET " + home,
		"GET " + home + "/files/tree",
		"GET " + home + "/chats",
		"GET " + home + "/threads",
	} {
		_, ok := got[r]
		assert.Truef(t, ok, "home route must be untouched by this step: %s", r)
	}

	_, ok := got["GET "+ws+"/threads"]
	assert.True(t, ok, "the threads group's workspace-scoped mount must survive")
}

func TestRouteAudit_NoWorkspaceScopedGroupRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	got := registeredRoutes(t)

	const repo = "/v0/projects/:projectId/repos/:repoId"
	const ws = repo + "/workspaces/:wsId"
	deleted := []string{
		"GET " + ws + "/files/tree",
		"GET " + ws + "/files/content",
		"PUT " + ws + "/files/content",
		"POST " + ws + "/files",
		"PATCH " + ws + "/files",
		"DELETE " + ws + "/files",
		"POST " + ws + "/files/copy",
		"GET " + ws + "/files/ws",
		"GET " + ws + "/blame",
		"POST " + ws + "/lsp/completion",
		"POST " + ws + "/lsp/hover",
		"POST " + ws + "/lsp/definition",
		"POST " + ws + "/lsp/references",
		"POST " + ws + "/lsp/rename",
		"POST " + ws + "/lsp/codeAction",
		"POST " + ws + "/lsp/documentSymbol",
		"GET " + ws + "/lsp/diagnostics",
		"POST " + ws + "/lsp/didOpen",
		"POST " + ws + "/lsp/didChange",
		"POST " + ws + "/lsp/didClose",
		"GET " + ws + "/lsp/ws",
		"GET " + ws + "/git/status",
		"GET " + ws + "/git/log",
		"GET " + ws + "/git/diff",
		"GET " + ws + "/git/branches",
		"GET " + ws + "/git/stashes",
		"GET " + ws + "/git/conflicts",
		"GET " + ws + "/git/conflict-hunks",
		"GET " + ws + "/git/commit-diff",
		"POST " + ws + "/git/stage",
		"POST " + ws + "/git/stage-hunk",
		"POST " + ws + "/git/unstage",
		"POST " + ws + "/git/unstage-hunk",
		"POST " + ws + "/git/discard",
		"POST " + ws + "/git/commit",
		"POST " + ws + "/git/push",
		"POST " + ws + "/git/pull",
		"POST " + ws + "/git/fetch",
		"POST " + ws + "/git/branches",
		"PATCH " + ws + "/git/branches",
		"DELETE " + ws + "/git/branches",
		"POST " + ws + "/git/switch",
		"POST " + ws + "/git/stash",
		"POST " + ws + "/git/stash-apply",
		"POST " + ws + "/git/stash-pop",
		"DELETE " + ws + "/git/stash",
		"POST " + ws + "/git/reset",
		"POST " + ws + "/git/merge",
		"POST " + ws + "/git/rebase",
		"POST " + ws + "/git/resolve-hunk",
		"POST " + ws + "/git/operation/continue",
		"POST " + ws + "/git/operation/abort",
		"GET " + ws + "/review",
		"PATCH " + ws + "/review",
		"GET " + ws + "/review/files",
		"GET " + ws + "/review/outline",
		"GET " + ws + "/review/patch",
		"GET " + ws + "/review/search",
		"POST " + ws + "/search",
		"POST " + ws + "/search/replace",
		"GET " + ws + "/identity",
		"GET " + ws + "/provider",
	}
	require.Len(t, deleted, 62, "the seven groups' sixty-two :wsId routes left to check; all must be")
	for _, r := range deleted {
		_, ok := got[r]
		assert.Falsef(t, ok, "deleted workspace-scoped group route is still registered: %s", r)
	}

	for _, r := range []string{
		"GET " + ws + "/threads",
		"POST " + ws + "/threads",
		"GET " + ws + "/threads/:threadId",
		"PATCH " + ws + "/threads/:threadId",
		"DELETE " + ws + "/threads/:threadId",
		"POST " + ws + "/threads/:threadId/replies",
		"PATCH " + ws + "/threads/:threadId/messages/:messageId",
		"DELETE " + ws + "/threads/:threadId/messages/:messageId",
	} {
		_, ok := got[r]
		assert.Truef(t, ok, "threads' workspace-scoped route must survive: %s", r)
	}

	_, ok := got["GET "+repo+"/protected-branches"]
	assert.True(t, ok, "protected-branches must remain repo-scoped")

	const home = "/v0/projects/:projectId/home"
	_, ok = got["GET "+home+"/files/tree"]
	assert.True(t, ok, "home's own files surface must be untouched by this step")
}

func TestRouteAudit_LiveReadRoutesAnswerREST(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	seedWorkspace(t, tc, "w1")
	tc.app.Usecases.Worktree = stubChatWorktreeResolver{chatToWs: map[string]string{"chat-1": "w1"}, workspaces: tc.app.Repositories.Workspace}
	c := v0.New(tc.app, tc.eng, accessfake.Closed())
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	for _, path := range []string{
		"/v0/projects",
		"/v0/projects/p1",
		"/v0/projects/p1/repos",
		"/v0/projects/p1/repos/r1",
		"/v0/chats/chat-1/git/status",
	} {
		resp, err := http.Get(srv.URL + path)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		assert.NotEqual(t, http.StatusSwitchingProtocols, resp.StatusCode, "plain GET must serve REST: %s", path)

		conn, wsResp, err := websocket.DefaultDialer.Dial("ws"+srv.URL[len("http"):]+path, nil)
		if wsResp != nil {
			_ = wsResp.Body.Close()
		}
		if conn != nil {
			_ = conn.Close()
		}
		assert.Errorf(t, err, "no lifecycle socket upgrades at %s", path)
	}
}
