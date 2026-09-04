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
// flat chat-scoped group (spec §7.1), the only surface review is addressable
// through: /v0/chats/:chatId/review... . Thread CRUD lives on the
// first-class /threads endpoint (W9).
//
// review is spec §4.2's shared bucket: the worktree answers once, and every
// chat holding it gets that answer, resolved from the request context by
// chatScoped's own resolveChatWorktree middleware (see
// handlers.Handlers.workspaceID).
//
// The old /projects/:projectId/repos/:repoId/workspaces/:wsId/review... mount
// is gone (spec §8 step 6): every caller had already moved to the mount kept
// here.
func Register(
	chatScoped *gin.RouterGroup,
	reviewUsecase reviewhandlers.ReviewUsecase,
) {
	h := reviewhandlers.New(reviewUsecase)
	mount(chatScoped, "/review", h)
}

// mount registers the 6-route review surface under prefix on rg.
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
