// Package projects mounts the v0 project REST routes.
package projects

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	projecthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects/handlers"
)

// Register mounts the project list, detail, import, and delete routes on the
// supplied router group, backed by the project read, import, and delete
// usecases. The list and detail GET routes are dual-served: a plain GET answers
// REST while a WebSocket upgrade is routed to projectsWS (the Broadcaster
// [ProjectDTO] handle) for the live stream — a list-scope subscriber receives
// all projects, a :projectId-scope subscriber receives only that project (W7-2).
func Register(
	rg *gin.RouterGroup,
	reader projecthandlers.ListGetter,
	importer projecthandlers.Importer,
	deleter projecthandlers.Deleter,
	broadcast func(dto.ProjectDTO),
	projectsWS gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := projecthandlers.New(reader, importer, deleter, broadcast)
	rg.GET("/projects", dispatch(h.List, projectsWS))
	rg.POST("/projects", h.Import)
	rg.GET("/projects/:projectId", dispatch(h.Detail, projectsWS))
	rg.PATCH("/projects/:projectId", h.Patch)
	rg.DELETE("/projects/:projectId", h.Delete)
	// The icon routes mirror the repo ones a level up, minus /icon/github: a
	// project has no origin remote to read an owner avatar from. See icon.go.
	rg.GET("/projects/:projectId/icon", h.Icon)
	rg.PUT("/projects/:projectId/icon", h.PutIcon)
	rg.DELETE("/projects/:projectId/icon", h.DeleteIcon)
	rg.PUT("/projects/:projectId/icon/emoji", h.PutIconEmoji)
}
