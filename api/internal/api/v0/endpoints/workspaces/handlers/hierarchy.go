package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// mergeRequest is the POST .../workspaces/:wsId/merge-into-parent body: the
// merge strategy to apply when folding the child branch into its parent.
type mergeRequest struct {
	Strategy gitdomain.MergeStrategy `json:"strategy"`
}

// reparentRequest is the POST .../workspaces/:wsId/reparent body: the id of the
// new parent the leaf child is rebased onto.
type reparentRequest struct {
	NewParentID string `json:"newParentId"`
}

// MergeIntoParent handles
// POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/merge-into-parent.
// It validates synchronously (body shape, strategy present, workspace exists)
// returning 4xx on failure; then it returns 202 and runs the local child→parent
// merge in the background. The merge outcome is delivered on the workspace
// WebSocket stream via the repository's broadcast callback; a failure surfaces as
// LastError on the entity (00 §4).
func (h *Handlers) MergeIntoParent(
	c *gin.Context,
) {
	var body mergeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Strategy == "" {
		libs.WriteErr(c, http.StatusBadRequest, "strategy is required")
		return
	}
	id := c.Param("wsId")
	if _, err := h.reader.Get(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteAccepted(c)
	runAsync(
		c.Request.Context(),
		h.broadcastLastError,
		id,
		func(ctx context.Context) error {
			_, mergeErr := h.hierarchy.MergeIntoParent(ctx, id, body.Strategy)
			return mergeErr
		},
	)
}

// Reparent handles
// POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/reparent. It
// validates synchronously (body shape, newParentId present, workspace exists)
// returning 4xx on failure; then it returns 202 and rebases the leaf child onto
// the new parent in the background. The reparented workspace is delivered on the
// workspace WebSocket stream via the repository's broadcast callback; a failure
// surfaces as LastError on the entity (00 §4).
func (h *Handlers) Reparent(
	c *gin.Context,
) {
	var body reparentRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.NewParentID == "" {
		libs.WriteErr(c, http.StatusBadRequest, "newParentId is required")
		return
	}
	id := c.Param("wsId")
	if _, err := h.reader.Get(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteAccepted(c)
	runAsync(
		c.Request.Context(),
		h.broadcastLastError,
		id,
		func(ctx context.Context) error {
			_, reparentErr := h.hierarchy.Reparent(ctx, id, body.NewParentID)
			return reparentErr
		},
	)
}
