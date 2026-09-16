// Package reqscope carries the per-request scope a v0 routing middleware
// resolved, from the middleware that resolved it to the handlers mounted below
// it.
//
// It exists as its own package because the two sides cannot see each other:
// internal/api/v0 imports every endpoint group (router.go), so an endpoint's
// handlers importing v0 back to read a context key would be an import cycle.
// A leaf package both sides may import is the only shape that works, and
// keeping the key unexported here is what stops a handler from reaching into
// the gin context with a hand-written string and drifting from the middleware.
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

// Workspace returns the workspace resolveChatWorktree resolved for this
// request, and whether one was actually stashed. Every handler mounted under
// rg.Group("/chats/:chatId") (v0/router.go) calls this rather than resolving
// the chat's worktree a second time — the resolve is the middleware's job, and
// doing it twice per request is what mounting it as middleware avoids.
//
// A false second return means the handler is mounted somewhere the middleware
// does not run, which is a wiring bug, not a request the caller can fix.
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
