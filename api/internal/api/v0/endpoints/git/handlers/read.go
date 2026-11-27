package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

func (h *Handlers) Status(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := h.workspaceID(ctx)

	status, err := h.git.Status(rctx, id)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, dto.GitStatusDTOFrom(status))
}

func (h *Handlers) Diff(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := h.workspaceID(ctx)

	stagedStr := ctx.DefaultQuery("staged", "false")
	staged, err := strconv.ParseBool(stagedStr)
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, "staged must be true or false")
		return
	}

	diffs, err := h.git.Diff(rctx, id, staged)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, diffs)
}

func (h *Handlers) Log(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := h.workspaceID(ctx)

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
		libs.Fail(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, commits)
}

func (h *Handlers) Branches(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := h.workspaceID(ctx)

	branches, err := h.git.Branches(rctx, id)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, branches)
}

func (h *Handlers) Stashes(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := h.workspaceID(ctx)

	stashes, err := h.git.Stashes(rctx, id)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, stashes)
}

func (h *Handlers) Conflicts(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := h.workspaceID(ctx)

	files, err := h.git.ConflictedFiles(rctx, id)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, files)
}

func (h *Handlers) ConflictHunks(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := h.workspaceID(ctx)

	filePath := ctx.Query("path")
	if filePath == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path query parameter is required")
		return
	}

	hunks, err := h.git.ConflictHunks(rctx, id, filePath)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, hunks)
}

func (h *Handlers) CommitDiff(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := h.workspaceID(ctx)

	sha := ctx.Query("sha")
	if sha == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "sha query parameter is required")
		return
	}

	diff, err := h.git.CommitDiff(rctx, id, sha)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, diff)
}
