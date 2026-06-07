package handlers

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// createRequest is the POST /v0/workspaces/:wsId/chats body: an optional title
// for the new root chat. An empty body is tolerated and yields an untitled chat.
type createRequest struct {
	Title string `json:"title"`
}

// renameRequest is the PATCH /v0/chats/:id body: the required new title.
type renameRequest struct {
	Title string `json:"title"`
}

// Create handles POST /v0/workspaces/:wsId/chats, minting a root chat for the
// workspace and returning the created id with status 201. The title is optional.
func (h *Handlers) Create(
	c *gin.Context,
) {
	var body createRequest
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.chats.CreateChat(
		c.Request.Context(),
		c.Param("wsId"),
		body.Title,
		h.now(),
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusCreated, created.ID)
}

// Fork handles POST /v0/chats/:id/fork, forking a child chat from the parent's
// current tip and returning the child's id with status 201.
func (h *Handlers) Fork(
	c *gin.Context,
) {
	forked, err := h.chats.ForkChat(c.Request.Context(), c.Param("id"), h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusCreated, forked.ID)
}

// Rename handles PATCH /v0/chats/:id, renaming a chat. The title is required and
// a missing or empty title yields 400.
func (h *Handlers) Rename(
	c *gin.Context,
) {
	var body renameRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Title == "" {
		libs.WriteErr(c, http.StatusBadRequest, "title is required")
		return
	}
	renamed, err := h.chats.RenameChat(c.Request.Context(), c.Param("id"), body.Title)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, renamed.ID)
}

// Delete handles DELETE /v0/chats/:id, cascade-deleting the chat and its
// descendants and returning the requested id.
func (h *Handlers) Delete(
	c *gin.Context,
) {
	id := c.Param("id")
	if err := h.chats.DeleteChat(c.Request.Context(), id, h.now()); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, id)
}
