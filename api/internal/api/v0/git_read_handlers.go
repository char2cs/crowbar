package v0

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// registerGitReadHandlers mounts the read-only git REST routes on rg.
func registerGitReadHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.GET("/workspaces/:wsId/git/status", c.handleGitStatus)
	rg.GET("/workspaces/:wsId/git/diff", c.handleGitDiff)
	rg.GET("/workspaces/:wsId/git/log", c.handleGitLog)
	rg.GET("/workspaces/:wsId/git/blame", c.handleGitBlame)
	rg.GET("/workspaces/:wsId/git/branches", c.handleGitBranches)
	rg.GET("/workspaces/:wsId/git/stashes", c.handleGitStashes)
	rg.GET("/workspaces/:wsId/git/conflicts", c.handleGitConflicts)
	rg.GET("/workspaces/:wsId/git/conflict-hunks", c.handleGitConflictHunks)
	rg.GET("/workspaces/:wsId/git/commit-diff", c.handleGitCommitDiff)
}

// handleGitStatus GET /v0/workspaces/:wsId/git/status
func (c *Container) handleGitStatus(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	status, err := c.app.Usecases.Git.Status(rctx, id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, status)
}

// handleGitDiff GET /v0/workspaces/:wsId/git/diff?staged=true|false
func (c *Container) handleGitDiff(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	stagedStr := ctx.DefaultQuery("staged", "false")
	staged, err := strconv.ParseBool(stagedStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "staged must be true or false"})
		return
	}

	diffs, err := c.app.Usecases.Git.Diff(rctx, id, staged)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, diffs)
}

// handleGitLog GET /v0/workspaces/:wsId/git/log?limit=50&skip=0
func (c *Container) handleGitLog(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	limitStr := ctx.DefaultQuery("limit", "50")
	skipStr := ctx.DefaultQuery("skip", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "limit must be an integer"})
		return
	}

	skip, err := strconv.Atoi(skipStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "skip must be an integer"})
		return
	}

	commits, err := c.app.Usecases.Git.Log(rctx, id, limit, skip)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, commits)
}

// handleGitBlame GET /v0/workspaces/:wsId/git/blame?path=<filePath>
func (c *Container) handleGitBlame(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	filePath := ctx.Query("path")
	if filePath == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter is required"})
		return
	}

	entries, err := c.app.Usecases.Git.Blame(rctx, id, filePath)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, entries)
}

// handleGitBranches GET /v0/workspaces/:wsId/git/branches
func (c *Container) handleGitBranches(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	branches, err := c.app.Usecases.Git.Branches(rctx, id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, branches)
}

// handleGitStashes GET /v0/workspaces/:wsId/git/stashes
func (c *Container) handleGitStashes(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	stashes, err := c.app.Usecases.Git.Stashes(rctx, id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, stashes)
}

// handleGitConflicts GET /v0/workspaces/:wsId/git/conflicts
func (c *Container) handleGitConflicts(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	files, err := c.app.Usecases.Git.ConflictedFiles(rctx, id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, files)
}

// handleGitConflictHunks GET /v0/workspaces/:wsId/git/conflict-hunks?path=<filePath>
func (c *Container) handleGitConflictHunks(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	filePath := ctx.Query("path")
	if filePath == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter is required"})
		return
	}

	hunks, err := c.app.Usecases.Git.ConflictHunks(rctx, id, filePath)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, hunks)
}

// handleGitCommitDiff GET /v0/workspaces/:wsId/git/commit-diff?sha=<sha>
func (c *Container) handleGitCommitDiff(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	sha := ctx.Query("sha")
	if sha == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "sha query parameter is required"})
		return
	}

	diff, err := c.app.Usecases.Git.CommitDiff(rctx, id, sha)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, diff)
}
