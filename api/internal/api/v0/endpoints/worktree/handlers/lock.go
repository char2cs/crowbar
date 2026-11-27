package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

type lockRequest struct {
	Locked *bool `json:"locked"`
}

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
