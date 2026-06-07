package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// createBranchRequest is the POST branches body: the new branch name and an
// optional source ref to branch from (defaulting to the current HEAD).
type createBranchRequest struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// renameBranchRequest is the PATCH branches/:branch body: the new name the
// branch is renamed to.
type renameBranchRequest struct {
	NewName string `json:"newName"`
}

// checkoutRequest is the POST checkout body: the branch to switch the working
// tree to.
type checkoutRequest struct {
	Branch string `json:"branch"`
}

// CreateBranch handles POST /v0/workspaces/:wsId/git/branches, creating a branch
// from the optional source ref. name is required; a missing name yields 400. The
// branch is not switched to. The response echoes the workspace id with status
// 201, since the route creates a ref.
func (h *Handlers) CreateBranch(
	c *gin.Context,
) {
	var body createBranchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name == "" {
		libs.WriteErr(c, http.StatusBadRequest, "name is required")
		return
	}
	err := h.git.CreateBranch(c.Request.Context(), c.Param("wsId"), body.Name, body.Source, false, h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusCreated, c.Param("wsId"))
}

// RenameBranch handles PATCH /v0/workspaces/:wsId/git/branches/:branch, renaming
// the path branch to newName. The :branch param is URL-decoded by gin (branch
// names may contain "/"). newName is required; a missing value yields 400. The
// response echoes the workspace id.
func (h *Handlers) RenameBranch(
	c *gin.Context,
) {
	var body renameBranchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.NewName == "" {
		libs.WriteErr(c, http.StatusBadRequest, "newName is required")
		return
	}
	err := h.git.RenameBranch(c.Request.Context(), c.Param("wsId"), c.Param("branch"), body.NewName, h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}

// DeleteBranch handles DELETE /v0/workspaces/:wsId/git/branches/:branch,
// deleting the path branch. The :branch param is URL-decoded by gin. The
// response echoes the workspace id.
func (h *Handlers) DeleteBranch(
	c *gin.Context,
) {
	err := h.git.DeleteBranch(c.Request.Context(), c.Param("wsId"), c.Param("branch"), h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}

// Checkout handles POST /v0/workspaces/:wsId/git/checkout, switching the working
// tree to branch. branch is required; a missing value yields 400. The response
// echoes the workspace id.
func (h *Handlers) Checkout(
	c *gin.Context,
) {
	var body checkoutRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Branch == "" {
		libs.WriteErr(c, http.StatusBadRequest, "branch is required")
		return
	}
	err := h.git.SwitchBranch(c.Request.Context(), c.Param("wsId"), body.Branch, h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}
