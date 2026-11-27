package reqscope

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/domain"
)

const chatWorktreeKey = "v0.chatWorktree"

func SetWorkspace(
	c *gin.Context,
	ws domain.Workspace,
) {
	c.Set(chatWorktreeKey, ws)
}

func Workspace(
	c *gin.Context,
) (domain.Workspace, bool) {
	v, ok := c.Get(chatWorktreeKey)
	if !ok {
		return domain.Workspace{}, false
	}
	ws, ok := v.(domain.Workspace)
	return ws, ok
}
