package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Status GET /v0/workspaces/:wsId/git/status
func (h *Handlers) Status(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	status, err := h.git.Status(rctx, id)
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	// Serialise through the DTO so a clean tree yields files: [] (never null).
	libs.WriteQueryOK(ctx, dto.GitStatusDTOFrom(status))
}

// Diff GET /v0/workspaces/:wsId/git/diff?staged=true|false
func (h *Handlers) Diff(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	stagedStr := ctx.DefaultQuery("staged", "false")
	staged, err := strconv.ParseBool(stagedStr)
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, "staged must be true or false")
		return
	}

	diffs, err := h.git.Diff(rctx, id, staged)
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	libs.WriteQueryOK(ctx, diffs)
}

// Log GET /v0/workspaces/:wsId/git/log?limit=50&skip=0
func (h *Handlers) Log(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	limitStr := ctx.DefaultQuery("limit", "50")
	skipStr := ctx.DefaultQuery("skip", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, "limit must be an integer")
		return
	}

	skip, err := strconv.Atoi(skipStr)
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, "skip must be an integer")
		return
	}

	commits, err := h.git.Log(rctx, id, limit, skip)
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	libs.WriteQueryOK(ctx, commits)
}

// Blame GET /v0/workspaces/:wsId/git/blame?path=<filePath>
func (h *Handlers) Blame(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	filePath := ctx.Query("path")
	if filePath == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path query parameter is required")
		return
	}

	entries, err := h.git.Blame(rctx, id, filePath)
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	libs.WriteQueryOK(ctx, entries)
}

// Branches GET /v0/workspaces/:wsId/git/branches
func (h *Handlers) Branches(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	branches, err := h.git.Branches(rctx, id)
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	libs.WriteQueryOK(ctx, branches)
}

// Stashes GET /v0/workspaces/:wsId/git/stashes
func (h *Handlers) Stashes(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	stashes, err := h.git.Stashes(rctx, id)
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	libs.WriteQueryOK(ctx, stashes)
}

// Conflicts GET /v0/workspaces/:wsId/git/conflicts
func (h *Handlers) Conflicts(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	files, err := h.git.ConflictedFiles(rctx, id)
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	libs.WriteQueryOK(ctx, files)
}

// ConflictHunks GET /v0/workspaces/:wsId/git/conflict-hunks?path=<filePath>
func (h *Handlers) ConflictHunks(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	filePath := ctx.Query("path")
	if filePath == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path query parameter is required")
		return
	}

	hunks, err := h.git.ConflictHunks(rctx, id, filePath)
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	libs.WriteQueryOK(ctx, hunks)
}

// CommitDiff GET /v0/workspaces/:wsId/git/commit-diff?sha=<sha>
func (h *Handlers) CommitDiff(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("wsId")

	sha := ctx.Query("sha")
	if sha == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "sha query parameter is required")
		return
	}

	diff, err := h.git.CommitDiff(rctx, id, sha)
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	libs.WriteQueryOK(ctx, diff)
}
