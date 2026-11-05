package v0

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// repoScopeReader loads a repository row by id for the repo scope guard.
type repoScopeReader interface {
	FindByKey(ctx context.Context, id string) (*domain.Repository, error)
}

// scopeRepoToPath 404s a :repoId that doesn't belong to the URL's :projectId, as for a
// missing id. Repo handlers load by :repoId alone but act on :projectId, so without it a
// caller could delete or read any repo from an unrelated project path.
func scopeRepoToPath(reader repoScopeReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		repoID := c.Param("repoId")
		if repoID == "" {
			c.Next()
			return
		}
		repo, err := reader.FindByKey(c.Request.Context(), repoID)
		if err != nil {
			libs.Fail(c, err)
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

// workspaceScopeReader loads a workspace row by id for the scope guard.
type workspaceScopeReader interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}

// scopeWorkspaceToPath 404s a :wsId that doesn't belong to the URL's :projectId/:repoId,
// as for a missing id. Handlers load by :wsId alone, so without it a stale URL could
// silently mutate the wrong workspace.
func scopeWorkspaceToPath(reader workspaceScopeReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		wsID := c.Param("wsId")
		if wsID == "" {
			c.Next()
			return
		}
		ws, err := reader.Get(c.Request.Context(), wsID)
		if err != nil {
			libs.Fail(c, err)
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

// chatWorktreeResolver resolves a chat id to the workspace whose worktree it reads and
// writes through; usecases.WorktreeResolver satisfies it.
type chatWorktreeResolver interface {
	Resolve(ctx context.Context, chatID string) (domain.Workspace, error)
}

// resolveChatWorktree resolves the workspace behind a /chats/:chatId route's chat
// ancestry once and stashes it in reqscope (a leaf package, to avoid an import cycle).
// Any resolve failure is a 404: an unresolvable chat is indistinguishable from a missing one.
func resolveChatWorktree(resolver chatWorktreeResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatID := c.Param("chatId")
		ws, err := resolver.Resolve(c.Request.Context(), chatID)
		if err != nil {
			libs.WriteErr(c, http.StatusNotFound, "chat not found")
			c.Abort()
			return
		}
		reqscope.SetWorkspace(c, ws)
		c.Next()
	}
}

// rejectEmptyPathParams 400s any empty path parameter: gin binds the empty segment in
// "//" to a param, and handlers would otherwise treat "" as a real id.
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
