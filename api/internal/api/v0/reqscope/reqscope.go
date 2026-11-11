// Package reqscope carries the per-request scope a v0 middleware resolved to the
// handlers below it. It is a leaf package because v0 imports every endpoint group, so
// handlers importing v0 would cycle; the unexported key stops hand-written lookups.
package reqscope

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// chatWorktreeKey is the gin context key SetWorkspace stashes the resolved
// domain.Workspace under. Unexported on purpose: Workspace is the only way to
// read it back.
const chatWorktreeKey = "v0.chatWorktree"

// SetWorkspace records the workspace a chat-scoped request resolved to. Called
// once per request by v0's resolveChatWorktree middleware.
func SetWorkspace(
	c *gin.Context,
	ws domain.Workspace,
) {
	c.Set(chatWorktreeKey, ws)
}

// Workspace returns the workspace resolveChatWorktree resolved for this request, so
// /chats/:chatId handlers never resolve it twice. false means the handler is mounted
// where the middleware does not run: a wiring bug, not a caller error.
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
