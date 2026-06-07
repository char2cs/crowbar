package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Get handles GET /v0/workspaces/:wsId/review, assembling the composite
// branch-review read model (description, merge strategy, range diff, comment
// threads, and read-only chat conversations) for the workspace. The data field
// carries the BranchReviewDTO.
func (h *Handlers) Get(
	c *gin.Context,
) {
	review, err := h.review.Get(
		c.Request.Context(),
		c.Param("wsId"),
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.BranchReviewDTOFrom(review))
}

// setMergeStrategyBody is the JSON body of the merge-strategy PATCH: the merge
// strategy to apply to the workspace.
type setMergeStrategyBody struct {
	MergeStrategy *gitdomain.MergeStrategy `json:"mergeStrategy"`
}

// SetMergeStrategy handles PATCH /v0/workspaces/:wsId/review, updating the
// workspace's merge strategy. The data field echoes the applied strategy under
// { "mergeStrategy": <strategy> }.
func (h *Handlers) SetMergeStrategy(
	c *gin.Context,
) {
	var body setMergeStrategyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.MergeStrategy == nil {
		libs.WriteErr(c, http.StatusBadRequest, "mergeStrategy is required")
		return
	}
	if err := h.review.SetMergeStrategy(
		c.Request.Context(),
		c.Param("wsId"),
		*body.MergeStrategy,
	); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, gin.H{"mergeStrategy": *body.MergeStrategy})
}
