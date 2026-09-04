package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// lockRequest is the POST /v0/.../workspaces/:wsId/lock body.
//
// `locked` is a POINTER because omitting it is a third, meaningful answer: it
// clears the user's override and hands the question back to the provider, so a
// branch that was manually unlocked goes back to being locked iff it is
// protected. See domain.Workspace.LockOverride.
type lockRequest struct {
	Locked *bool `json:"locked"`
}

// Lock handles POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lock.
//
// It answers SYNCHRONOUSLY, like the hierarchy PATCH and unlike the git-backed
// operations around it: this is one aggregate write with no git in it, and every
// way it can be refused — the workspace is the project home, or a placeholder
// with no worktree of its own — is something the user has to see while the menu
// they pressed it from is still in front of them. A 202 would strand those
// behind a LastError frame.
//
// The updated workspace still arrives on the workspaces WebSocket stream, so the
// sidebar does not patch its own cache from this response.
func (h *Handlers) Lock(
	c *gin.Context,
) {
	h.lock(c, h.namedWorkspace)
}

// ChatLock handles POST .../repos/:repoId/chats/:id/lock — spec §4.3's
// chat-keyed twin of Lock, and the same handler body reached the other way.
func (h *Handlers) ChatLock(
	c *gin.Context,
) {
	h.lock(c, h.chatWorkspace)
}

// lock is the body both keyings share. The BIND runs before the workspace is
// resolved, so a malformed body is refused identically whichever way the
// request named its target — and, as before, never reaches the store.
func (h *Handlers) lock(
	c *gin.Context,
	target workspaceTarget,
) {
	var body lockRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	id, ok := target(c)
	if !ok {
		return
	}
	if _, err := h.reader.SetLock(c.Request.Context(), id, body.Locked); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	c.Status(http.StatusNoContent)
}
