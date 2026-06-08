package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// State handles GET /v0/workspaces/:wsId/provider.
// Runs PollOnView for the workspace and returns its ProviderState JSON.
// When capability is disabled, returns ProviderState{Protected: false, PR: nil}.
func (h *Handlers) State(
	ctx *gin.Context,
) {
	wsID := ctx.Param("wsId")
	ws, err := h.wsReader.Get(ctx.Request.Context(), wsID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	pollCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()

	state, err := h.eng.PollOnView(
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

// ProtectedBranches handles GET /v0/repos/:id/protected-branches.
// Looks up any workspace for this repo to obtain the WorktreePath, then
// returns the list of protected branch names. When capability is disabled,
// returns the DefaultProtectedBranches fallback.
// The :id parameter is the workspace ID whose worktree path is used as the
// repo root (Wave 3 will wire the proper repo aggregate; for now we reuse
// the workspace lookup that is already wired).
func (h *Handlers) ProtectedBranches(
	ctx *gin.Context,
) {
	wsID := ctx.Param("id")
	ws, err := h.wsReader.Get(ctx.Request.Context(), wsID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	branches, err := h.eng.ProtectedBranches(ctx.Request.Context(), ws.WorktreePath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "provider: protected-branches error for ws %s: %v\n", wsID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "provider poll failed"})
		return
	}

	ctx.JSON(http.StatusOK, branches)
}
