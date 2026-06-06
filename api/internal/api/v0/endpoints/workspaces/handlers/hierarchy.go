package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// mergeRequest is the POST /v0/workspaces/:childId/merge-into-parent body: the
// merge strategy to apply when folding the child branch into its parent.
type mergeRequest struct {
	Strategy gitdomain.MergeStrategy `json:"strategy"`
}

// mergeResponse is the merge-into-parent result body: whether the merge stalled
// on conflicts (leaving a pending-merge marker) and, on a clean merge, the
// parent's post-merge tip SHA.
type mergeResponse struct {
	ConflictsPending bool   `json:"conflictsPending"`
	ParentTipSha     string `json:"parentTipSha,omitempty"`
}

// reparentRequest is the POST /v0/workspaces/:childId/reparent body: the id of
// the new parent the leaf child is rebased onto.
type reparentRequest struct {
	NewParentID string `json:"newParentId"`
}

// MergeIntoParent handles POST /v0/workspaces/:childId/merge-into-parent,
// running a local child→parent merge with the requested strategy.
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
	result, err := h.hierarchy.MergeIntoParent(
		c.Request.Context(),
		c.Param("wsId"),
		body.Strategy,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, mergeResponse{
		ConflictsPending: result.ConflictsPending,
		ParentTipSha:     result.ParentTipSha,
	})
}

// Reparent handles POST /v0/workspaces/:childId/reparent, rebasing a leaf child
// onto a new parent and returning the updated WorkspaceDTO.
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
	updated, err := h.hierarchy.Reparent(
		c.Request.Context(),
		c.Param("wsId"),
		body.NewParentID,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.WorkspaceDTOFrom(updated))
}
