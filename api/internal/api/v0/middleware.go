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

type repoScopeReader interface {
	FindByKey(ctx context.Context, id string) (*domain.Repository, error)
}

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

type workspaceScopeReader interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}

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

type chatWorktreeResolver interface {
	Resolve(ctx context.Context, chatID string) (domain.Workspace, error)
}

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
