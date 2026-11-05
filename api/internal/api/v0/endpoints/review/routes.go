// Package review mounts the v0 branch-review REST routes: the review read model, the windowed
// diff API and the merge-strategy mutation. Review threads live in endpoints/threads.
package review

import (
	"github.com/gin-gonic/gin"

	reviewhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review/handlers"
)

// Register mounts the review routes on the chat-scoped group (/v0/chats/:chatId/review...). Review
// is per worktree: every chat holding it gets the same answer, resolved by chatScoped's
// resolveChatWorktree middleware.
func Register(
	chatScoped *gin.RouterGroup,
	reviewUsecase reviewhandlers.ReviewUsecase,
) {
	h := reviewhandlers.New(reviewUsecase)
	mount(chatScoped, "/review", h)
}

// mount registers the review surface under prefix on rg.
func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *reviewhandlers.Handlers,
) {
	rg.GET(prefix, h.Get)
	rg.GET(prefix+"/files", h.GetFiles)
	// The windowed diff API: hunk geometry, one file's patch and server-side find-in-diff,
	// so a huge branch diff is never materialised in one payload.
	rg.GET(prefix+"/outline", h.GetOutline)
	rg.GET(prefix+"/patch", h.GetPatch)
	rg.GET(prefix+"/search", h.SearchDiff)
	// The whole text of one file on each side, fetched only when the reader asks
	// to reveal the unchanged lines between hunks.
	rg.GET(prefix+"/contents", h.GetFileSides)
	rg.PATCH(prefix, h.SetMergeStrategy)
}
