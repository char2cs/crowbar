package v0

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// repoScopeReader loads a repository row by id for the repo scope guard.
type repoScopeReader interface {
	FindByKey(ctx context.Context, id string) (*domain.Repository, error)
}

// scopeRepoToPath enforces that a request's :repoId belongs to the :projectId in
// the SAME URL — the project-level analogue of scopeWorkspaceToPath. The repo
// handlers (Detail/Delete/Icon*/Branches) load a repo by :repoId alone and act on
// the URL :projectId (DeleteRepo even os.RemoveAll's repoDir(home, projectId,
// repoId)), so without this a caller could delete, re-icon, or read ANY repo by id
// from an unrelated project path — cross-project destruction. We load the repo
// once and 404 on a project mismatch (the same response a missing id gets). Routes
// with no :repoId (the repo collection list/create) pass through.
//
// Workspace-scoped requests carry :repoId too and are validated here as well as by
// scopeWorkspaceToPath downstream; the extra read is a cheap read-model lookup and
// keeps the guard independent of the workspace↔repo data invariant.
func scopeRepoToPath(reader repoScopeReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		repoID := c.Param("repoId")
		if repoID == "" {
			c.Next()
			return
		}
		repo, err := reader.FindByKey(c.Request.Context(), repoID)
		if err != nil {
			status, msg := libs.StatusAndMessage(err)
			libs.WriteErr(c, status, msg)
			c.Abort()
			return
		}
		if repo == nil || repo.ProjectID != c.Param("projectId") {
			libs.WriteErr(c, http.StatusNotFound, "repository not found")
			c.Abort()
			return
		}
		c.Next()
	}
}

// workspaceScopeReader loads a workspace row by id for the scope guard. It is
// the narrow surface scopeWorkspaceToPath needs; the concrete workspace repo
// satisfies it.
type workspaceScopeReader interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}

// scopeWorkspaceToPath enforces that a request's :wsId actually belongs to the
// :projectId/:repoId in the SAME URL.
//
// Every entity-scoped route lives under
// /projects/:projectId/repos/:repoId/workspaces/:wsId/... but the handlers below
// load a workspace by :wsId alone (reader.Get(wsId)). Without this guard the
// :projectId/:repoId segments are decorative: a caller could read or mutate ANY
// workspace by id from an unrelated project/repo path — a wrong-scope confusion
// that turns a stale or malformed URL into a silent wrong-workspace mutation
// (delete/merge/reparent) or cross-scope read. We load the workspace once and
// 404 on any project/repo mismatch — the same response a genuinely missing id
// gets, so scope is never probed by id. Routes with no :wsId (collection
// list/create, repo-level reads) pass straight through.
func scopeWorkspaceToPath(reader workspaceScopeReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		wsID := c.Param("wsId")
		if wsID == "" {
			c.Next()
			return
		}
		ws, err := reader.Get(c.Request.Context(), wsID)
		if err != nil {
			status, msg := libs.StatusAndMessage(err)
			libs.WriteErr(c, status, msg)
			c.Abort()
			return
		}
		if ws.ProjectID != c.Param("projectId") || ws.RepoID != c.Param("repoId") {
			libs.WriteErr(c, http.StatusNotFound, "workspace not found")
			c.Abort()
			return
		}
		c.Next()
	}
}

// chatWorktreeContextKey is the gin context key resolveChatWorktree stashes
// the resolved domain.Workspace under. Every handler mounted below chatScoped
// (router.go) reads it back with WorkspaceFromContext instead of resolving
// the chat's worktree a second time.
const chatWorktreeContextKey = "v0.chatWorktree"

// WorkspaceFromContext returns the domain.Workspace resolveChatWorktree
// resolved for this request, and whether one was actually stashed. Every
// handler mounted under rg.Group("/chats/:chatId") (router.go) calls this
// rather than re-resolving the chat's worktree itself.
func WorkspaceFromContext(
	c *gin.Context,
) (domain.Workspace, bool) {
	v, ok := c.Get(chatWorktreeContextKey)
	if !ok {
		return domain.Workspace{}, false
	}
	ws, ok := v.(domain.Workspace)
	return ws, ok
}

// chatWorktreeResolver resolves a chat id to the workspace whose worktree it
// reads and writes through (internal/app/usecases/worktree, spec
// docs/superpowers/specs/2026-09-02-chat-scoped-api-design.md §3). Declared
// here rather than imported from usecases/worktree directly (law 4) — it is
// the narrow Resolve(ctx, chatID) shape resolveChatWorktree needs; the
// container's Worktree resolver (usecases.WorktreeResolver) satisfies it
// structurally.
type chatWorktreeResolver interface {
	Resolve(ctx context.Context, chatID string) (domain.Workspace, error)
}

// resolveChatWorktree scopes every route mounted under
// rg.Group("/chats/:chatId") (router.go, spec §7.1's flat chat prefix) to the
// workspace behind the chat's worktree, and stashes it on the gin context
// (chatWorktreeContextKey) so every handler below reads it back without a
// second resolve call per request.
//
// Unlike scopeWorkspaceToPath, which loads a workspace named directly by the
// URL and only validates it against sibling :projectId/:repoId segments, a
// chat is never itself a workspace: the workspace has to be resolved from the
// chat's ancestry (spec §3) before any handler below can run. A chat with no
// worktree anywhere in its ancestry (worktree.ErrNoWorktreeInAncestry) and
// any other resolve failure both write the same 404-shaped envelope
// scopeWorkspaceToPath uses for an unscoped id — from the caller's
// perspective a chat whose worktree cannot be resolved is indistinguishable
// from one that doesn't exist.
func resolveChatWorktree(resolver chatWorktreeResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatID := c.Param("chatId")
		ws, err := resolver.Resolve(c.Request.Context(), chatID)
		if err != nil {
			libs.WriteErr(c, http.StatusNotFound, "chat not found")
			c.Abort()
			return
		}
		c.Set(chatWorktreeContextKey, ws)
		c.Next()
	}
}

// rejectEmptyPathParams guards every v0 route against empty path parameters.
//
// gin's radix tree happily matches a request like GET /v0/workspaces//chats
// against /v0/workspaces/:wsId/chats with wsId == "" — the empty segment
// between the two slashes binds the param. Handlers would then pass "" to
// repositories and usecases as if it were a real id (list queries return data
// scoped to a nonexistent workspace, lookups 404 confusingly, etc.). Reject
// such requests up front with a 400 error envelope.
func rejectEmptyPathParams() gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, p := range c.Params {
			if p.Value == "" {
				libs.WriteErr(
					c,
					http.StatusBadRequest,
					fmt.Sprintf("path parameter %q must not be empty", p.Key),
				)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
