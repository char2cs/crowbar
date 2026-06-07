// Package search mounts the v0 global search and search-and-replace REST routes
// (02 §2.6). Both routes hang off /workspaces/:wsId: search reads the worktree
// and returns a match list, while replace rewrites occurrences on disk and
// honours the workspace Locked flag. Backed by the search engine and the
// workspace reader.
package search

import (
	"github.com/gin-gonic/gin"

	searchhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search/handlers"
)

// Register mounts the search and replace routes on the supplied router group,
// backed by the search engine and the workspace reader.
func Register(
	rg *gin.RouterGroup,
	eng searchhandlers.SearchEngine,
	wsReader searchhandlers.WorkspaceReader,
) {
	h := searchhandlers.New(eng, wsReader)
	rg.POST("/workspaces/:wsId/search", h.Search)
	rg.POST("/workspaces/:wsId/search/replace", h.Replace)
}
