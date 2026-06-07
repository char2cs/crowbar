package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// resetRequest is the POST reset body: the reset mode ("soft", "mixed", or
// "hard") and the target commit HEAD is moved to.
type resetRequest struct {
	Mode   string `json:"mode"`
	Commit string `json:"commit"`
}

// mergeRequest is the POST merge body: the branch merged into the current
// branch.
type mergeRequest struct {
	Branch string `json:"branch"`
}

// rebaseRequest is the POST rebase body: the ref the current branch is rebased
// onto.
type rebaseRequest struct {
	Onto string `json:"onto"`
}

func (r resetRequest) validMode() bool {
	return r.Mode == "soft" || r.Mode == "mixed" || r.Mode == "hard"
}

// Reset handles POST /v0/workspaces/:wsId/git/reset, moving HEAD to commit with
// the given mode. mode must be "soft", "mixed", or "hard" and commit is
// required; an invalid mode or missing commit yields 400. The response echoes
// the workspace id.
func (h *Handlers) Reset(
	c *gin.Context,
) {
	var body resetRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if !body.validMode() {
		libs.WriteErr(c, http.StatusBadRequest, "mode must be soft, mixed, or hard")
		return
	}
	if body.Commit == "" {
		libs.WriteErr(c, http.StatusBadRequest, "commit is required")
		return
	}
	err := h.git.Reset(c.Request.Context(), c.Param("wsId"), body.Mode, body.Commit, h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}

// Merge handles POST /v0/workspaces/:wsId/git/merge, merging branch into the
// current branch. branch is required; a missing value yields 400. The response
// echoes the workspace id.
func (h *Handlers) Merge(
	c *gin.Context,
) {
	var body mergeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Branch == "" {
		libs.WriteErr(c, http.StatusBadRequest, "branch is required")
		return
	}
	err := h.git.Merge(c.Request.Context(), c.Param("wsId"), body.Branch, h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}

// Rebase handles POST /v0/workspaces/:wsId/git/rebase, rebasing the current
// branch onto onto. onto is required; a missing value yields 400. The response
// echoes the workspace id.
func (h *Handlers) Rebase(
	c *gin.Context,
) {
	var body rebaseRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Onto == "" {
		libs.WriteErr(c, http.StatusBadRequest, "onto is required")
		return
	}
	err := h.git.Rebase(c.Request.Context(), c.Param("wsId"), body.Onto, h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}
