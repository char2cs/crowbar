// Package threads mounts the first-class workspace-scoped review-thread routes:
// the thread list/open (GET/WS + POST), detail/resolve (GET/PATCH), and reply
// (POST). Threads were promoted out of the /review surface into their own
// entity-scoped endpoint + WebSocket topic (09, 00 §5.5).
package threads

import (
	"github.com/gin-gonic/gin"

	threadhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/threads/handlers"
)

// Register mounts the thread routes on the repo-scoped group. The thread list
// (GET .../threads) is dual-served: a plain GET lists the workspace's threads,
// while a WebSocket upgrade is routed to the thread broadcaster (spec §5). POST
// opens a thread; GET/PATCH .../:threadId read/resolve it; POST .../:threadId/
// replies appends a reply. Every mutation broadcasts the resulting ThreadDTO.
func Register(
	repoScoped *gin.RouterGroup,
	store threadhandlers.ThreadStore,
	broadcast threadhandlers.ThreadBroadcaster,
	wsHandle gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := threadhandlers.New(store, broadcast)
	rg := repoScoped.Group("/workspaces/:wsId")
	rg.GET("/threads", dispatch(h.List, wsHandle))
	rg.POST("/threads", h.OpenThread)
	rg.GET("/threads/:threadId", h.Detail)
	rg.PATCH("/threads/:threadId", h.SetResolved)
	rg.POST("/threads/:threadId/replies", h.Reply)
}
