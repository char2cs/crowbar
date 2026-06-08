// Package search mounts the v0 global search and replace REST routes.
package search

import (
	"github.com/gin-gonic/gin"

	searchhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search/handlers"
)

// Register mounts the search and replace routes on the supplied router group.
func Register(
	rg *gin.RouterGroup,
	searchEng searchhandlers.SearchEngine,
	wsReader searchhandlers.WorkspaceReader,
) {
	h := searchhandlers.New(searchEng, wsReader)
	rg.POST("/workspaces/:wsId/search", h.Search)
	rg.POST("/workspaces/:wsId/search/replace", h.Replace)
}
