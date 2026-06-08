package v0

import (
	"net/http"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func registerRepoHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.POST("/repos", c.handleRepoCreate)
	rg.GET("/repos/:id", c.handleRepoGet)
}

func (c *Container) handleRepoCreate(
	ctx *gin.Context,
) {
	var body struct {
		ID            string `json:"id"`
		ProjectID     string `json:"projectId"`
		Name          string `json:"name"`
		Path          string `json:"path"`
		DefaultBranch string `json:"defaultBranch"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defaultBranch := body.DefaultBranch
	if defaultBranch == "" && body.Path != "" {
		defaultBranch = gitDefaultBranch(body.Path)
	}
	repo := domain.Repository{
		ID:            body.ID,
		ProjectID:     body.ProjectID,
		Name:          body.Name,
		Path:          body.Path,
		DefaultBranch: defaultBranch,
	}
	if err := c.app.GORM.Repositories.Save(ctx.Request.Context(), repo); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, repo)
}

// gitDefaultBranch reads the current branch from a git repository at path.
// Returns "" if path is not a git repo or the command fails.
func gitDefaultBranch(
	path string,
) string {
	out, err := exec.Command("git", "-C", path, "symbolic-ref", "HEAD", "--short").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (c *Container) handleRepoGet(
	ctx *gin.Context,
) {
	repo, err := c.app.GORM.Repositories.FindByKey(ctx.Request.Context(), ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, repo)
}
