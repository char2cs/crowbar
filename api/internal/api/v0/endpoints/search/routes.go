// Package search mounts the v0 global search and replace REST routes.
package search

import (
	"github.com/gin-gonic/gin"

	searchhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search/handlers"
)

// Register mounts the search and replace routes on the flat chat-scoped
// group (spec §7.1), the only surface search is addressable through:
// /v0/chats/:chatId/search... .
//
// search is spec §4.2's shared bucket: the worktree answers once, and every
// chat holding it gets that answer, resolved from the request context by
// chatScoped's own resolveChatWorktree middleware (see
// handlers.Handlers.resolveWorkspace).
//
// The old /projects/:projectId/repos/:repoId/workspaces/:wsId/search... mount
// is gone (spec §8 step 6): every caller had already moved to the mount kept
// here.
func Register(
	chatScoped *gin.RouterGroup,
	searchEng searchhandlers.SearchEngine,
) {
	h := searchhandlers.New(searchEng)
	mount(chatScoped, "/search", h)
}

// mount registers the 2-route search surface under prefix on rg.
func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *searchhandlers.Handlers,
) {
	rg.POST(prefix, h.Search)
	rg.POST(prefix+"/replace", h.Replace)
}
