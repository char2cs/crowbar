package handlers

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// Sync handles POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/sync.
// It validates the workspace exists synchronously (4xx if not), then returns 202
// and recomputes the working-tree summary in the background. The synced
// workspace is delivered on the workspace WebSocket stream via the repository's
// broadcast callback; a failure surfaces as LastError on the entity (00 §4).
func (h *Handlers) Sync(
	c *gin.Context,
) {
	h.sync(c, h.namedWorkspace)
}

// ChatSync handles POST .../repos/:repoId/chats/:id/sync — spec §4.3's
// chat-keyed twin of Sync, and the same handler body reached the other way.
func (h *Handlers) ChatSync(
	c *gin.Context,
) {
	h.sync(c, h.chatWorkspace)
}

// sync is the body both keyings share.
func (h *Handlers) sync(
	c *gin.Context,
	target workspaceTarget,
) {
	id, ok := target(c)
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
