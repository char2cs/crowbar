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
	rg.GET("/workspaces/:wsId/review/files", h.GetFiles)
	// The windowed diff API. /review carries the whole diff in one O(lines)
	// payload; these three describe and serve it in pieces no one of which is —
	// the hunk geometry of every file, one file's patch, and a server-side
	// find-in-diff — so a million-line branch is never materialised anywhere.
	rg.GET("/workspaces/:wsId/review/outline", h.GetOutline)
	rg.GET("/workspaces/:wsId/review/patch", h.GetPatch)
	rg.GET("/workspaces/:wsId/review/search", h.SearchDiff)
	rg.PATCH("/workspaces/:wsId/review", h.SetMergeStrategy)
}
