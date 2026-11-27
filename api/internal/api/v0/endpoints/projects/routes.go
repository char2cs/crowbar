package projects

import (
	"github.com/gin-gonic/gin"

	projecthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects/handlers"
)

func Register(
	rg *gin.RouterGroup,
	reader projecthandlers.ListGetter,
	importer projecthandlers.Importer,
	deleter projecthandlers.Deleter,
	ops projecthandlers.Operations,
	crowbarHome func() (string, error),
) {
	h := projecthandlers.New(reader, importer, deleter, ops, crowbarHome)
	rg.GET("/projects", h.List)
	rg.POST("/projects", h.Import)
	rg.GET("/projects/:projectId", h.Detail)
	rg.PATCH("/projects/:projectId", h.Patch)
	rg.DELETE("/projects/:projectId", h.Delete)
	rg.GET("/projects/:projectId/icon", h.Icon)
	rg.PUT("/projects/:projectId/icon", h.PutIcon)
	rg.DELETE("/projects/:projectId/icon", h.DeleteIcon)
	rg.PUT("/projects/:projectId/icon/emoji", h.PutIconEmoji)
}
