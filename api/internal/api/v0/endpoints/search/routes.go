// Package search mounts the v0 global search and replace REST routes.
package search

import (
	"github.com/gin-gonic/gin"

	searchhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search/handlers"
)

// Register mounts the search and replace routes on BOTH scoping groups
// search is currently addressable through.
//
// search is spec §4.2's shared bucket: the worktree answers once, and every
// chat holding it gets that answer. chatScoped is where that lives from now
// on — /v0/chats/:chatId/search... , the flat prefix §7.1 closes on — and the
// frontend talks to it exclusively.
//
// wsScoped is the OLD /projects/:projectId/repos/:repoId/workspaces/:wsId/
// search... surface, mounted unchanged. It is not a fallback and nothing
// chooses between the two: it is simply a route that has not been retired
// yet, and retiring it is spec §8 step 6's job, once every group has moved
// and the workspaces/home groups are deleted wholesale. Deleting THIS call is
// the whole of search's share of that step.
//
// One route table serves both, so the two can never drift into different
// surfaces: mount is called twice with different prefixes, and a route added
// to it appears on both by construction. The handlers themselves take the
// workspace from whichever mount the request arrived on — see
// handlers.Handlers.resolveWorkspace.
func Register(
	wsScoped *gin.RouterGroup,
	chatScoped *gin.RouterGroup,
	searchEng searchhandlers.SearchEngine,
	wsReader searchhandlers.WorkspaceReader,
) {
	h := searchhandlers.New(searchEng, wsReader)
	mount(chatScoped, "/search", h)
	mount(wsScoped, "/workspaces/:wsId/search", h)
}

// mount registers the 2-route search surface under prefix on rg. It is the
// single definition of that surface; Register calls it once per live scoping
// group.
func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *searchhandlers.Handlers,
) {
	rg.POST(prefix, h.Search)
	rg.POST(prefix+"/replace", h.Replace)
}
