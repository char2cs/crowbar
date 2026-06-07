// Package review mounts the v0 branch-review REST routes: the composite review
// read model, the merge-strategy update, and the comment thread mutations
// (open, reply, resolve/reopen) (02 §2.9, 09). Every route hangs off
// /workspaces/:wsId/review; the thread mutations carry their target thread id in
// the :id path param.
package review

import (
	"github.com/gin-gonic/gin"

	reviewhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review/handlers"
)

// Register mounts the branch-review read, merge-strategy, and comment-thread
// routes on the supplied router group, backed by the branch-review usecase.
func Register(
	rg *gin.RouterGroup,
	review reviewhandlers.ReviewService,
) {
	h := reviewhandlers.New(review)
	rg.GET("/workspaces/:wsId/review", h.Get)
	rg.PATCH("/workspaces/:wsId/review", h.SetMergeStrategy)
	rg.POST("/workspaces/:wsId/review/threads", h.OpenThread)
	rg.POST("/workspaces/:wsId/review/threads/:id/reply", h.Reply)
	rg.PATCH("/workspaces/:wsId/review/threads/:id", h.SetThreadResolved)
}
