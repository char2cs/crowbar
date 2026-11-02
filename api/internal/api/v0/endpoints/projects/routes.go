// Package projects mounts the v0 project REST routes.
package projects

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	projecthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects/handlers"
)

// Register mounts the project list, detail, import, and delete routes on the
// supplied router group, backed by the project read, import, and delete
// usecases. broadcast publishes each project a route changed.
func Register(
	rg *gin.RouterGroup,
	reader projecthandlers.ListGetter,
	importer projecthandlers.Importer,
	deleter projecthandlers.Deleter,
	ops projecthandlers.Operations,
	broadcast func(dto.ProjectDTO),
	crowbarHome func() (string, error),
) {
	h := projecthandlers.New(reader, importer, deleter, ops, broadcast, crowbarHome)
	rg.GET("/projects", h.List)
	rg.POST("/projects", h.Import)
	rg.GET("/projects/:projectId", h.Detail)
	rg.PATCH("/projects/:projectId", h.Patch)
	rg.DELETE("/projects/:projectId", h.Delete)
	// The icon routes mirror the repo ones a level up, minus /icon/github: a
	// project has no origin remote to read an owner avatar from. See icon.go.
	rg.GET("/projects/:projectId/icon", h.Icon)
	rg.PUT("/projects/:projectId/icon", h.PutIcon)
	rg.DELETE("/projects/:projectId/icon", h.DeleteIcon)
	rg.PUT("/projects/:projectId/icon/emoji", h.PutIconEmoji)
}
