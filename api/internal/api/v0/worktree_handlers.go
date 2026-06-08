package v0

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// registerWorktreeHandlers mounts the worktree hierarchy REST routes on rg.
func registerWorktreeHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.POST("/workspaces/:wsId/child", c.handleWorktreeCreateChild)
	rg.POST("/workspaces/:wsId/merge", c.handleWorktreeMerge)
	rg.POST("/workspaces/:wsId/reparent", c.handleWorktreeReparent)
	rg.DELETE("/workspaces/:wsId/cascade", c.handleWorktreeDeleteCascade)
}

// handleWorktreeCreateChild POST /v0/workspaces/:wsId/child
// Creates a new worktree-backed child workspace under the workspace identified
// by :id. The body supplies repo/project/branch metadata; parentId is taken
// from the path parameter.
func (c *Container) handleWorktreeCreateChild(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()

	var body struct {
		RepoID       string `json:"repoId"`
		ProjectID    string `json:"projectId"`
		RepoPath     string `json:"repoPath"`
		Branch       string `json:"branch"`
		ParentBranch string `json:"parentBranch"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	parentID := ctx.Param("wsId")

	ws, err := c.app.Usecases.Worktree.CreateChild(reqCtx, worktree.CreateChildInput{
		RepoID:       body.RepoID,
		ProjectID:    body.ProjectID,
		RepoPath:     body.RepoPath,
		Branch:       body.Branch,
		ParentID:     parentID,
		ParentBranch: body.ParentBranch,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, ws)
}

// handleWorktreeMerge POST /v0/workspaces/:wsId/merge
// Merges the child workspace identified by :id into its parent using the
// supplied strategy ("merge", "rebase", or "squash"). Returns the MergeResult,
// which includes a ConflictsPending flag when git reported conflicts.
func (c *Container) handleWorktreeMerge(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()

	var body struct {
		Strategy string `json:"strategy"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	childID := ctx.Param("wsId")

	result, err := c.app.Usecases.Worktree.MergeIntoParent(
		reqCtx,
		childID,
		gitdomain.MergeStrategy(body.Strategy),
	)
	if err != nil {
		if isWorktreeBusinessError(err) {
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// handleWorktreeReparent POST /v0/workspaces/:wsId/reparent
// Moves the workspace identified by :id under a new parent workspace.
// The body must supply newParentId.
func (c *Container) handleWorktreeReparent(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()

	var body struct {
		NewParentID string `json:"newParentId"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	childID := ctx.Param("wsId")

	ws, err := c.app.Usecases.Worktree.Reparent(reqCtx, childID, body.NewParentID)
	if err != nil {
		if isWorktreeBusinessError(err) {
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, ws)
}

// isWorktreeBusinessError returns true when err is a domain rule violation that
// the client triggered intentionally (as opposed to an infrastructure failure).
// These map to 422 rather than 500.
func isWorktreeBusinessError(
	err error,
) bool {
	return errors.Is(err, worktree.ErrChildHasChildren) ||
		errors.Is(err, worktree.ErrParentLocked) ||
		errors.Is(err, worktree.ErrNewParentLocked) ||
		errors.Is(err, worktree.ErrRebaseNonLeaf)
}

// handleWorktreeDeleteCascade DELETE /v0/workspaces/:wsId/cascade
// Cascades deletion of the workspace subtree rooted at :id, removing all
// descendant worktrees and branches before removing the root itself.
func (c *Container) handleWorktreeDeleteCascade(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()

	rootID := ctx.Param("wsId")

	if err := c.app.Usecases.Worktree.DeleteCascade(reqCtx, rootID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}
