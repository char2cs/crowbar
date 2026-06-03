package v0

import (
	"github.com/char2cs/crowbar/api/internal/fixtures"
	"github.com/char2cs/crowbar/api/internal/wshub"
	"github.com/gin-gonic/gin"
)

type WSGitHandler struct {
	hub   *wshub.Hub
	store *fixtures.Store
}

func NewWSGitHandler(hub *wshub.Hub, store *fixtures.Store) *WSGitHandler {
	return &WSGitHandler{hub: hub, store: store}
}

func (h *WSGitHandler) Upgrade(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.hub.Register(conn)
	defer func() {
		h.hub.Unregister(conn)
		conn.Close()
	}()

	repo := c.Query("repo")
	_ = conn.WriteJSON(fixtures.GitEvent{Repo: repo, Changed: false})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
