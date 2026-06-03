package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

type FsHandler struct{ store *fixtures.Store }

func NewFsHandler(store *fixtures.Store) *FsHandler {
	return &FsHandler{store: store}
}

func (h *FsHandler) Tree(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.FileTree)
}
