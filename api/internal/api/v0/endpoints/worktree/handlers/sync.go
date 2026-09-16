package handlers

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// ChatSync handles POST .../repos/:repoId/chats/:id/sync. It validates the chat
// resolves synchronously (4xx if not), then returns 202 and recomputes the
// working-tree summary in the background. The synced workspace is delivered on
// the workspace WebSocket stream via the repository's broadcast callback; a
// failure surfaces as LastError on the entity (00 §4).
func (h *Handlers) ChatSync(
	c *gin.Context,
) {
	id, ok := h.chatWorkspace(c)
	if !ok {
		return
	}
	libs.WriteAccepted(c)
	h.runAsync(
		c.Request.Context(),
		h.working,
		h.broadcastLastError,
		id,
		func(ctx context.Context) error {
			_, syncErr := h.reader.SyncWorkingTreeState(ctx, id, time.Now())
			return syncErr
		},
	)
}
