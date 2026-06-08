package v0

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func registerWorkspaceHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.POST("/workspaces", c.handleWorkspaceCreate)
	rg.GET("/workspaces", c.handleWorkspaceList)
	rg.GET("/workspaces/:wsId", c.handleWorkspaceGet)
	rg.DELETE("/workspaces/:wsId", c.handleWorkspaceDelete)
	rg.POST("/workspaces/:wsId/sync", c.handleWorkspaceSync)
	rg.PATCH("/workspaces/:wsId", c.handleWorkspacePatch)
}

func (c *Container) handleWorkspaceCreate(
	ctx *gin.Context,
) {
	var body struct {
		ID            string                   `json:"id"`
		RepoID        string                   `json:"repoId"`
		ProjectID     string                   `json:"projectId"`
		Branch        string                   `json:"branch"`
		WorktreePath  string                   `json:"worktreePath"`
		ForkPointSha  string                   `json:"forkPointSha"`
		ParentID      string                   `json:"parentId"`
		Locked        bool                     `json:"locked"`
		MergeStrategy gitdomain.MergeStrategy  `json:"mergeStrategy"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ws, err := c.app.Repositories.Workspace.Create(
		ctx.Request.Context(),
		workspace.CreateInput{
			ID:            body.ID,
			RepoID:        body.RepoID,
			ProjectID:     body.ProjectID,
			Branch:        body.Branch,
			WorktreePath:  body.WorktreePath,
			ForkPointSha:  body.ForkPointSha,
			ParentID:      body.ParentID,
			Locked:        body.Locked,
			MergeStrategy: body.MergeStrategy,
		},
		time.Now(),
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, ws)
}

func (c *Container) handleWorkspaceList(
	ctx *gin.Context,
) {
	list, err := c.app.Usecases.Workspace.List(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []domain.Workspace{}
	}
	ctx.JSON(http.StatusOK, list)
}

func (c *Container) handleWorkspaceGet(
	ctx *gin.Context,
) {
	ws, err := c.app.Usecases.Workspace.Get(ctx.Request.Context(), ctx.Param("wsId"))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, ws)
}

func (c *Container) handleWorkspaceDelete(
	ctx *gin.Context,
) {
	if err := c.app.Usecases.Worktree.DeleteCascade(ctx.Request.Context(), ctx.Param("wsId")); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (c *Container) handleWorkspaceSync(
	ctx *gin.Context,
) {
	ws, err := c.app.Usecases.Workspace.SyncWorkingTreeState(
		ctx.Request.Context(),
		ctx.Param("wsId"),
		time.Now(),
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, ws)
}

func (c *Container) handleWorkspacePatch(
	ctx *gin.Context,
) {
	var body struct {
		MergeStrategy gitdomain.MergeStrategy `json:"mergeStrategy"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ws, err := c.app.Usecases.Workspace.SetMergeStrategy(
		ctx.Request.Context(),
		ctx.Param("wsId"),
		body.MergeStrategy,
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, ws)
}
