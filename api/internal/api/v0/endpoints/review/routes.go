// Package review mounts the v0 branch-review REST routes: the composite review
// read model and merge-strategy mutation (02 §2.9, 09). Review threads were
// promoted out of this surface into the first-class workspace-scoped /threads
// endpoint + WebSocket topic (W9); see endpoints/threads.
package review

import (
	"github.com/gin-gonic/gin"

	reviewhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review/handlers"
)

// Register mounts the branch-review read and merge-strategy routes on BOTH
// scoping groups review is currently addressable through, backed by the
// branch-review usecase. Thread CRUD now lives on the first-class /threads
// endpoint (W9).
//
// review is spec §4.2's shared bucket: the worktree answers once, and every
// chat holding it gets that answer. chatScoped is where that lives from now
// on — /v0/chats/:chatId/review... , the flat prefix §7.1 closes on — and the
// frontend talks to it exclusively.
//
// wsScoped is the OLD /projects/:projectId/repos/:repoId/workspaces/:wsId/
// review... surface, mounted unchanged. It is not a fallback and nothing
// chooses between the two: it is simply a route that has not been retired
// yet, and retiring it is spec §8 step 6's job, once every group has moved
// and the workspaces/home groups are deleted wholesale. Deleting THIS call is
// the whole of review's share of that step.
//
// One route table serves both, so the two can never drift into different
// surfaces: mount is called twice with different prefixes, and a route added
// to it appears on both by construction. The handlers themselves take the
// worktree from whichever mount the request arrived on — see
// handlers.Handlers.workspaceID.
func Register(
	wsScoped *gin.RouterGroup,
	chatScoped *gin.RouterGroup,
	reviewUsecase reviewhandlers.ReviewUsecase,
) {
	h := reviewhandlers.New(reviewUsecase)
	mount(chatScoped, "/review", h)
	mount(wsScoped, "/workspaces/:wsId/review", h)
}

// mount registers the 6-route review surface under prefix on rg. It is the
// single definition of that surface; Register calls it once per live scoping
// group.
func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *reviewhandlers.Handlers,
) {
	rg.GET(prefix, h.Get)
	rg.GET(prefix+"/files", h.GetFiles)
	// The windowed diff API. /review carries the whole diff in one O(lines)
	// payload; these three describe and serve it in pieces no one of which is —
	// the hunk geometry of every file, one file's patch, and a server-side
	// find-in-diff — so a million-line branch is never materialised anywhere.
	rg.GET(prefix+"/outline", h.GetOutline)
	rg.GET(prefix+"/patch", h.GetPatch)
	rg.GET(prefix+"/search", h.SearchDiff)
	rg.PATCH(prefix, h.SetMergeStrategy)
}
