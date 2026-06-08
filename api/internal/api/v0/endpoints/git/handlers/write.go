package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Stage POST /workspaces/:wsId/git/stage
func (h *Handlers) Stage(
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

	if err := h.git.StageFile(reqCtx, wsID, body.Path, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// StageHunk POST /workspaces/:wsId/git/stage-hunk
func (h *Handlers) StageHunk(
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

	if err := h.git.StageHunk(reqCtx, wsID, body.Path, body.HunkID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// Unstage POST /workspaces/:wsId/git/unstage
func (h *Handlers) Unstage(
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

	if err := h.git.UnstageFile(reqCtx, wsID, body.Path, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// UnstageHunk POST /workspaces/:wsId/git/unstage-hunk
func (h *Handlers) UnstageHunk(
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

	if err := h.git.UnstageHunk(reqCtx, wsID, body.Path, body.HunkID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// Discard POST /workspaces/:wsId/git/discard
func (h *Handlers) Discard(
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

	if err := h.git.Discard(reqCtx, wsID, body.Path, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// Commit POST /workspaces/:wsId/git/commit
func (h *Handlers) Commit(
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

	if err := h.git.Commit(reqCtx, wsID, body.Message, body.Author, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// Push POST /workspaces/:wsId/git/push
func (h *Handlers) Push(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	if err := h.git.Push(reqCtx, wsID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// Fetch POST /workspaces/:wsId/git/fetch
func (h *Handlers) Fetch(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	if err := h.git.Fetch(reqCtx, wsID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// Pull POST /workspaces/:wsId/git/pull
func (h *Handlers) Pull(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	if err := h.git.Pull(reqCtx, wsID, "", time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// CreateBranch POST /workspaces/:wsId/git/branches
func (h *Handlers) CreateBranch(
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

	if err := h.git.CreateBranch(reqCtx, wsID, body.Name, "", false, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusCreated)
}

// RenameBranch PATCH /workspaces/:wsId/git/branches
func (h *Handlers) RenameBranch(
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

	if err := h.git.RenameBranch(reqCtx, wsID, body.From, body.To, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// DeleteBranch DELETE /workspaces/:wsId/git/branches
func (h *Handlers) DeleteBranch(
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

	if err := h.git.DeleteBranch(reqCtx, wsID, body.Name, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}

// Switch POST /workspaces/:wsId/git/switch
func (h *Handlers) Switch(
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

	if err := h.git.SwitchBranch(reqCtx, wsID, body.Branch, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// StashPush POST /workspaces/:wsId/git/stash
func (h *Handlers) StashPush(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Message string `json:"message"`
	}
	// message is optional — ignore bind error
	_ = ctx.ShouldBindJSON(&body)

	if err := h.git.StashPush(reqCtx, wsID, body.Message, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// StashApply POST /workspaces/:wsId/git/stash-apply
func (h *Handlers) StashApply(
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
	if err := h.git.StashApply(reqCtx, wsID, stashID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// StashPop POST /workspaces/:wsId/git/stash-pop
func (h *Handlers) StashPop(
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
	if err := h.git.StashPop(reqCtx, wsID, stashID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// StashDrop DELETE /workspaces/:wsId/git/stash
func (h *Handlers) StashDrop(
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
	if err := h.git.StashDrop(reqCtx, wsID, stashID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}

// Reset POST /workspaces/:wsId/git/reset
func (h *Handlers) Reset(
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

	if err := h.git.Reset(reqCtx, wsID, body.Mode, "", time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// Merge POST /workspaces/:wsId/git/merge
func (h *Handlers) Merge(
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

	if err := h.git.Merge(reqCtx, wsID, body.Branch, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// Rebase POST /workspaces/:wsId/git/rebase
func (h *Handlers) Rebase(
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

	if err := h.git.Rebase(reqCtx, wsID, body.Branch, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// ResolveHunk POST /workspaces/:wsId/git/resolve-hunk
func (h *Handlers) ResolveHunk(
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
	if err := h.git.ResolveHunk(
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

// OperationContinue POST /workspaces/:wsId/git/operation/continue
func (h *Handlers) OperationContinue(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	if err := h.git.OperationContinue(reqCtx, wsID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

// OperationAbort POST /workspaces/:wsId/git/operation/abort
func (h *Handlers) OperationAbort(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	if err := h.git.OperationAbort(reqCtx, wsID, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}
