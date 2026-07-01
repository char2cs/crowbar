// Package review mounts the v0 branch-review REST routes: the composite review
// read model and merge-strategy mutation (02 §2.9, 09). Review threads were
// promoted out of this surface into the first-class workspace-scoped /threads
// endpoint + WebSocket topic (W9); see endpoints/threads.
package review

import (
	"github.com/gin-gonic/gin"

	reviewhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review/handlers"
)

// Register mounts the branch-review read and merge-strategy routes on the
// supplied router group, backed by the branch-review usecase. Thread CRUD now
// lives on the first-class /threads endpoint (W9).
func Register(
	rg *gin.RouterGroup,
	reviewUsecase reviewhandlers.ReviewUsecase,
) {
	h := reviewhandlers.New(reviewUsecase)
	rg.GET("/workspaces/:wsId/review", h.Get)
	rg.PATCH("/workspaces/:wsId/review", h.SetMergeStrategy)
}
