// Package repos mounts the v0 repository REST routes.
package repos

import (
	"github.com/gin-gonic/gin"

	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
)

// Register mounts the repo list and detail routes on the supplied router group,
// backed by the repository GORM store.
func Register(
	rg *gin.RouterGroup,
	store repohandlers.Store,
) {
	h := repohandlers.New(store)
	rg.POST("/repos", h.Create)
	rg.GET("/repos", h.List)
	rg.GET("/repos/:id", h.Detail)
}
