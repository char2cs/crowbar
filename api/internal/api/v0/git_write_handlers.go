package v0

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// registerGitWriteHandlers mounts all mutating git REST routes on rg.
func registerGitWriteHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.POST("/workspaces/:wsId/git/stage", c.handleGitStage)
	rg.POST("/workspaces/:wsId/git/stage-hunk", c.handleGitStageHunk)
	rg.POST("/workspaces/:wsId/git/unstage", c.handleGitUnstage)
	rg.POST("/workspaces/:wsId/git/unstage-hunk", c.handleGitUnstageHunk)
	rg.POST("/workspaces/:wsId/git/discard", c.handleGitDiscard)
	rg.POST("/workspaces/:wsId/git/commit", c.handleGitCommit)
	rg.POST("/workspaces/:wsId/git/push", c.handleGitPush)
	rg.POST("/workspaces/:wsId/git/fetch", c.handleGitFetch)
	rg.POST("/workspaces/:wsId/git/pull", c.handleGitPull)
	rg.POST("/workspaces/:wsId/git/branches", c.handleGitCreateBranch)
	rg.PATCH("/workspaces/:wsId/git/branches", c.handleGitRenameBranch)
	rg.DELETE("/workspaces/:wsId/git/branches", c.handleGitDeleteBranch)
	rg.POST("/workspaces/:wsId/git/switch", c.handleGitSwitch)
	rg.POST("/workspaces/:wsId/git/stash", c.handleGitStashPush)
	rg.POST("/workspaces/:wsId/git/stash-apply", c.handleGitStashApply)
	rg.POST("/workspaces/:wsId/git/stash-pop", c.handleGitStashPop)
	rg.DELETE("/workspaces/:wsId/git/stash", c.handleGitStashDrop)
	rg.POST("/workspaces/:wsId/git/reset", c.handleGitReset)
	rg.POST("/workspaces/:wsId/git/merge", c.handleGitMerge)
	rg.POST("/workspaces/:wsId/git/rebase", c.handleGitRebase)
	rg.POST("/workspaces/:wsId/git/resolve-hunk", c.handleGitResolveHunk)
	rg.POST("/workspaces/:wsId/git/operation/continue", c.handleGitOperationContinue)
	rg.POST("/workspaces/:wsId/git/operation/abort", c.handleGitOperationAbort)
}

// handleGitStage POST /workspaces/:wsId/git/stage
func (c *Container) handleGitStage(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Path string `json:"path"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Path == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	if err := c.app.Usecases.Git.StageFile(reqCtx, wsID, body.Path, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitStageHunk POST /workspaces/:wsId/git/stage-hunk
func (c *Container) handleGitStageHunk(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Path   string `json:"path"`
		HunkID string `json:"hunkId"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Path == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	if body.HunkID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "hunkId is required"})
		return
	}

	if err := c.app.Usecases.Git.StageHunk(reqCtx, wsID, body.Path, body.HunkID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitUnstage POST /workspaces/:wsId/git/unstage
func (c *Container) handleGitUnstage(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Path string `json:"path"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Path == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	if err := c.app.Usecases.Git.UnstageFile(reqCtx, wsID, body.Path, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitUnstageHunk POST /workspaces/:wsId/git/unstage-hunk
func (c *Container) handleGitUnstageHunk(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Path   string `json:"path"`
		HunkID string `json:"hunkId"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Path == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	if body.HunkID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "hunkId is required"})
		return
	}

	if err := c.app.Usecases.Git.UnstageHunk(reqCtx, wsID, body.Path, body.HunkID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitDiscard POST /workspaces/:wsId/git/discard
func (c *Container) handleGitDiscard(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Path string `json:"path"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Path == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	if err := c.app.Usecases.Git.Discard(reqCtx, wsID, body.Path, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitCommit POST /workspaces/:wsId/git/commit
func (c *Container) handleGitCommit(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Message string `json:"message"`
		Author  string `json:"author"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Message == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}

	if err := c.app.Usecases.Git.Commit(reqCtx, wsID, body.Message, body.Author, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitPush POST /workspaces/:wsId/git/push
func (c *Container) handleGitPush(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	if err := c.app.Usecases.Git.Push(reqCtx, wsID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitFetch POST /workspaces/:wsId/git/fetch
func (c *Container) handleGitFetch(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	if err := c.app.Usecases.Git.Fetch(reqCtx, wsID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitPull POST /workspaces/:wsId/git/pull
func (c *Container) handleGitPull(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	if err := c.app.Usecases.Git.Pull(reqCtx, wsID, "", time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitCreateBranch POST /workspaces/:wsId/git/branches
func (c *Container) handleGitCreateBranch(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Name string `json:"name"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if err := c.app.Usecases.Git.CreateBranch(reqCtx, wsID, body.Name, "", false, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusCreated)
}

// handleGitRenameBranch PATCH /workspaces/:wsId/git/branches
func (c *Container) handleGitRenameBranch(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.From == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "from is required"})
		return
	}
	if body.To == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "to is required"})
		return
	}

	if err := c.app.Usecases.Git.RenameBranch(reqCtx, wsID, body.From, body.To, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitDeleteBranch DELETE /workspaces/:wsId/git/branches
func (c *Container) handleGitDeleteBranch(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Name string `json:"name"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil || body.Name == "" {
		body.Name = ctx.Query("name")
	}
	if body.Name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if err := c.app.Usecases.Git.DeleteBranch(reqCtx, wsID, body.Name, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}

// handleGitSwitch POST /workspaces/:wsId/git/switch
func (c *Container) handleGitSwitch(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Branch string `json:"branch"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Branch == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "branch is required"})
		return
	}

	if err := c.app.Usecases.Git.SwitchBranch(reqCtx, wsID, body.Branch, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitStashPush POST /workspaces/:wsId/git/stash
func (c *Container) handleGitStashPush(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Message string `json:"message"`
	}
	// message is optional — ignore bind error
	_ = ctx.ShouldBindJSON(&body)

	if err := c.app.Usecases.Git.StashPush(reqCtx, wsID, body.Message, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitStashApply POST /workspaces/:wsId/git/stash-apply
func (c *Container) handleGitStashApply(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Index int `json:"index"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	stashID := strconv.Itoa(body.Index)
	if err := c.app.Usecases.Git.StashApply(reqCtx, wsID, stashID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitStashPop POST /workspaces/:wsId/git/stash-pop
func (c *Container) handleGitStashPop(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Index int `json:"index"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	stashID := strconv.Itoa(body.Index)
	if err := c.app.Usecases.Git.StashPop(reqCtx, wsID, stashID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitStashDrop DELETE /workspaces/:wsId/git/stash
func (c *Container) handleGitStashDrop(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Index *int `json:"index"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil || body.Index == nil {
		if raw := ctx.Query("index"); raw != "" {
			idx, err := strconv.Atoi(raw)
			if err != nil {
				ctx.JSON(http.StatusBadRequest, gin.H{"error": "index must be an integer"})
				return
			}
			body.Index = &idx
		}
	}
	if body.Index == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "index is required"})
		return
	}

	stashID := strconv.Itoa(*body.Index)
	if err := c.app.Usecases.Git.StashDrop(reqCtx, wsID, stashID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}

// handleGitReset POST /workspaces/:wsId/git/reset
func (c *Container) handleGitReset(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Mode string `json:"mode"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Mode == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "mode is required"})
		return
	}

	if err := c.app.Usecases.Git.Reset(reqCtx, wsID, body.Mode, "", time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitMerge POST /workspaces/:wsId/git/merge
func (c *Container) handleGitMerge(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Branch string `json:"branch"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Branch == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "branch is required"})
		return
	}

	if err := c.app.Usecases.Git.Merge(reqCtx, wsID, body.Branch, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitRebase POST /workspaces/:wsId/git/rebase
func (c *Container) handleGitRebase(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Branch string `json:"branch"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Branch == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "branch is required"})
		return
	}

	if err := c.app.Usecases.Git.Rebase(reqCtx, wsID, body.Branch, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitResolveHunk POST /workspaces/:wsId/git/resolve-hunk
func (c *Container) handleGitResolveHunk(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Path            string `json:"path"`
		HunkIndex       int    `json:"hunkIndex"`
		Choice          string `json:"choice"`
		ResolvedContent string `json:"resolvedContent"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Path == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	if body.Choice == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "choice is required"})
		return
	}

	hunkID := strconv.Itoa(body.HunkIndex)
	resolution := gitdomain.ConflictResolution(body.Choice)
	if err := c.app.Usecases.Git.ResolveHunk(
		reqCtx,
		wsID,
		body.Path,
		hunkID,
		resolution,
		body.ResolvedContent,
		time.Now(),
	); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitOperationContinue POST /workspaces/:wsId/git/operation/continue
func (c *Container) handleGitOperationContinue(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	if err := c.app.Usecases.Git.OperationContinue(reqCtx, wsID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// handleGitOperationAbort POST /workspaces/:wsId/git/operation/abort
func (c *Container) handleGitOperationAbort(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	if err := c.app.Usecases.Git.OperationAbort(reqCtx, wsID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}
