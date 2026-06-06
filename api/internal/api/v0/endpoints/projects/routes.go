// Package projects mounts the v0 project REST routes.
package projects

import (
	"github.com/gin-gonic/gin"

	projecthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects/handlers"
)

// Register mounts the project list, detail, and import routes on the supplied
// router group, backed by the project read and import usecases.
func Register(
	rg *gin.RouterGroup,
	reader projecthandlers.ListGetter,
	importer projecthandlers.Importer,
) {
	h := projecthandlers.New(reader, importer)
	rg.GET("/projects", h.List)
	rg.POST("/projects", h.Import)
	rg.GET("/projects/:id", h.Detail)
}
