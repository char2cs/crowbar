package v0

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
)

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
