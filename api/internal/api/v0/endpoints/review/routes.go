// Package review mounts the v0 branch-review REST routes: the composite review
// read model, merge-strategy mutation, and review-thread CRUD (open, reply,
// resolve/reopen) (02 §2.9, 09).
package review

import (
	"github.com/gin-gonic/gin"

	reviewhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review/handlers"
)

// Register mounts the branch-review read, merge-strategy, and thread routes on
// the supplied router group, backed by the branch-review usecase.
func Register(
	rg *gin.RouterGroup,
	reviewUsecase reviewhandlers.ReviewUsecase,
) {
	h := reviewhandlers.New(reviewUsecase)
	rg.GET("/workspaces/:wsId/review", h.Get)
	rg.PATCH("/workspaces/:wsId/review", h.SetMergeStrategy)
	rg.POST("/workspaces/:wsId/review/threads", h.OpenThread)
	rg.POST("/workspaces/:wsId/review/threads/:id/reply", h.Reply)
	rg.PATCH("/workspaces/:wsId/review/threads/:id", h.SetThreadResolved)
}
