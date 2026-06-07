package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// stashPushRequest is the POST stash body: an optional message describing the
// stash entry.
type stashPushRequest struct {
	Message string `json:"message"`
}

// stashRestoreRequest is the POST stash/:id body: the restore mode, either
// "apply" (keep the stash) or "pop" (apply then drop).
type stashRestoreRequest struct {
	Mode string `json:"mode"`
}

func (r stashRestoreRequest) valid() bool {
	return r.Mode == "apply" || r.Mode == "pop"
}

// StashPush handles POST /v0/workspaces/:wsId/git/stash, stashing the current
// working-tree changes with the optional message. The response echoes the
// workspace id.
func (h *Handlers) StashPush(
	c *gin.Context,
) {
	var body stashPushRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	err := h.git.StashPush(c.Request.Context(), c.Param("wsId"), body.Message, h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}

// StashRestore handles POST /v0/workspaces/:wsId/git/stash/:id, restoring the
// path stash either with "apply" (keep) or "pop" (apply then drop). mode must be
// "apply" or "pop"; any other value yields 400. The response echoes the
// workspace id.
func (h *Handlers) StashRestore(
	c *gin.Context,
) {
	var body stashRestoreRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if !body.valid() {
		libs.WriteErr(c, http.StatusBadRequest, "mode must be apply or pop")
		return
	}
	if err := h.stashRestore(c, body); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}

func (h *Handlers) stashRestore(
	c *gin.Context,
	body stashRestoreRequest,
) error {
	wsID := c.Param("wsId")
	id := c.Param("id")
	if body.Mode == "pop" {
		return h.git.StashPop(c.Request.Context(), wsID, id, h.now())
	}
	return h.git.StashApply(c.Request.Context(), wsID, id, h.now())
}

// StashDrop handles DELETE /v0/workspaces/:wsId/git/stash/:id, removing the path
// stash without applying it. The response echoes the workspace id.
func (h *Handlers) StashDrop(
	c *gin.Context,
) {
	err := h.git.StashDrop(c.Request.Context(), c.Param("wsId"), c.Param("id"), h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}
