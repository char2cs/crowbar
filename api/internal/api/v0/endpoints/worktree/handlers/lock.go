package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// lockRequest is the POST .../chats/:id/lock body. Locked is a pointer because omitting it clears
// the user's override, so the branch is locked iff protected (see domain.Workspace.LockOverride).
type lockRequest struct {
	Locked *bool `json:"locked"`
}

// ChatLock handles POST .../chats/:id/lock synchronously: it is one aggregate write with no git, and
// its refusals (project home, placeholder) must reach the user while the menu is open. The updated
// workspace still arrives on the workspaces stream.
func (h *Handlers) ChatLock(
	c *gin.Context,
) {
	var body lockRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	ws, ok := h.chatWorkspace(c)
	if !ok {
		return
	}
	if _, err := h.reader.SetLock(c.Request.Context(), ws.ID, body.Locked); err != nil {
		libs.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
