package v0

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/engine/provider"
)

// registerProviderHandlers mounts the read-only provider REST routes on rg.
func registerProviderHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.GET("/workspaces/:wsId/provider", c.handleProviderState)
	rg.GET("/repos/:id/protected-branches", c.handleProtectedBranches)
}

// handleProviderState GET /v0/workspaces/:wsId/provider
// Runs PollOnView for the workspace and returns its ProviderState JSON.
// When capability is disabled, returns ProviderState{Protected: false, PR: nil}.
func (c *Container) handleProviderState(
	ctx *gin.Context,
) {
	eng := c.requireProviderEngine(ctx)
	if eng == nil {
		return
	}

	wsID := ctx.Param("wsId")
	ws, err := c.app.Repositories.Workspace.Get(ctx.Request.Context(), wsID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	pollCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()

	state, err := eng.PollOnView(
		pollCtx,
		wsID,
		ws.WorktreePath,
		ws.Branch,
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "provider: poll error for ws %s: %v\n", wsID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "provider poll failed"})
		return
	}

	ctx.JSON(http.StatusOK, state)
}

// handleProtectedBranches GET /v0/repos/:id/protected-branches
// Looks up any workspace for this repo to obtain the WorktreePath, then
// returns the list of protected branch names. When capability is disabled,
// returns the DefaultProtectedBranches fallback.
// The :id parameter is the workspace ID whose worktree path is used as the
// repo root (Wave 3 will wire the proper repo aggregate; for now we reuse
// the workspace lookup that is already wired).
func (c *Container) handleProtectedBranches(
	ctx *gin.Context,
) {
	eng := c.requireProviderEngine(ctx)
	if eng == nil {
		return
	}

	wsID := ctx.Param("id")
	ws, err := c.app.Repositories.Workspace.Get(ctx.Request.Context(), wsID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	branches, err := eng.ProtectedBranches(ctx.Request.Context(), ws.WorktreePath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "provider: protected-branches error for ws %s: %v\n", wsID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "provider poll failed"})
		return
	}

	ctx.JSON(http.StatusOK, branches)
}

// requireProviderEngine returns the engine or writes 503 and returns nil.
func (c *Container) requireProviderEngine(
	ctx *gin.Context,
) provider.Engine {
	if c.eng == nil || c.eng.Provider == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "provider engine not available"})
		return nil
	}
	return c.eng.Provider
}
