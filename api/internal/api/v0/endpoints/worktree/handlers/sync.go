package handlers

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatSync handles POST .../chats/:id/sync, answering 202 with the operation that recomputes the
// working-tree summary; the synced workspace arrives on the workspace stream.
func (h *Handlers) ChatSync(
	c *gin.Context,
) {
	ws, ok := h.chatWorkspace(c)
	if !ok {
		return
	}
	h.startOn(c, ws, domain.OpWorkspaceSync, "Sync "+ws.Branch, func(ctx context.Context) error {
		_, err := h.reader.SyncWorkingTreeState(ctx, ws.ID, time.Now())
		return err
	})
}
