package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// pullRequest is the POST pull body: the integration mode, either "merge" or
// "rebase".
type pullRequest struct {
	Mode string `json:"mode"`
}

func (r pullRequest) valid() bool {
	return r.Mode == "merge" || r.Mode == "rebase"
}

// Push handles POST /v0/workspaces/:wsId/git/push, pushing the current branch to
// its remote. The response echoes the workspace id.
func (h *Handlers) Push(
	c *gin.Context,
) {
	err := h.git.Push(c.Request.Context(), c.Param("wsId"), h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}

// Pull handles POST /v0/workspaces/:wsId/git/pull, integrating remote changes
// with the given mode. mode must be "merge" or "rebase"; any other value yields
// 400. The response echoes the workspace id.
func (h *Handlers) Pull(
	c *gin.Context,
) {
	var body pullRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if !body.valid() {
		libs.WriteErr(c, http.StatusBadRequest, "mode must be merge or rebase")
		return
	}
	err := h.git.Pull(c.Request.Context(), c.Param("wsId"), body.Mode, h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}

// Fetch handles POST /v0/workspaces/:wsId/git/fetch, fetching remote refs
// without integrating them. The response echoes the workspace id.
func (h *Handlers) Fetch(
	c *gin.Context,
) {
	err := h.git.Fetch(c.Request.Context(), c.Param("wsId"), h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}
