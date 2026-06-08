package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// registerProjectHandlers mounts the project read REST routes on rg.
func registerProjectHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.GET("/projects", c.handleProjectList)
	rg.GET("/projects/:id", c.handleProjectGet)
}

// handleProjectList GET /v0/projects
// Returns all projects as a JSON array.
func (c *Container) handleProjectList(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()

	projects, err := c.app.Usecases.Project.List(reqCtx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, projects)
}

// handleProjectGet GET /v0/projects/:id
// Returns the project with the given id, or 404 when not found.
func (c *Container) handleProjectGet(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()

	id := ctx.Param("id")

	project, err := c.app.Usecases.Project.Get(reqCtx, id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, project)
}
