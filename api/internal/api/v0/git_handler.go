package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

type GitHandler struct{ store *fixtures.Store }

func NewGitHandler(store *fixtures.Store) *GitHandler {
	return &GitHandler{store: store}
}

func (h *GitHandler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.GitStatus)
}

func (h *GitHandler) Log(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.GitLog)
}

func (h *GitHandler) Branches(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.GitBranches)
}
