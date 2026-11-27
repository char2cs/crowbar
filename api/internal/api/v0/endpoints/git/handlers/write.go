package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

type pathsBody struct {
	Paths []string `json:"paths"`
}

func bindPaths(
	ctx *gin.Context,
) ([]string, bool) {
	var body pathsBody
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return nil, false
	}
	if len(body.Paths) == 0 {
		libs.WriteErr(ctx, http.StatusBadRequest, "paths is required")
		return nil, false
	}
	for _, p := range body.Paths {
		if p == "" {
			libs.WriteErr(ctx, http.StatusBadRequest, "paths must not contain empty entries")
			return nil, false
		}
	}
	return body.Paths, true
}

func (h *Handlers) Stage(ctx *gin.Context) { h.eachPath(ctx, h.git.StageFile) }

func (h *Handlers) StageHunk(ctx *gin.Context) { h.oneHunk(ctx, h.git.StageHunk) }

func (h *Handlers) Unstage(ctx *gin.Context) { h.eachPath(ctx, h.git.UnstageFile) }

func (h *Handlers) UnstageHunk(ctx *gin.Context) { h.oneHunk(ctx, h.git.UnstageHunk) }

func (h *Handlers) Discard(ctx *gin.Context) { h.eachPath(ctx, h.git.Discard) }

func (h *Handlers) eachPath(
	ctx *gin.Context,
	op func(ctx context.Context, wsID, filePath string, now time.Time) error,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

	paths, ok := bindPaths(ctx)
	if !ok {
		return
	}

	now := time.Now()
	for _, p := range paths {
		if err := op(reqCtx, wsID, p, now); err != nil {
			libs.Fail(ctx, err)
			return
		}
	}

	libs.WriteMutationOK(ctx, http.StatusOK, wsID)
}

func (h *Handlers) oneHunk(
	ctx *gin.Context,
	op func(ctx context.Context, wsID, filePath, hunkID string, now time.Time) error,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

	var body struct {
		Path   string `json:"path"`
		HunkID string `json:"hunkId"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.Path == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path is required")
		return
	}
	if body.HunkID == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "hunkId is required")
		return
	}

	if err := op(reqCtx, wsID, body.Path, body.HunkID, time.Now()); err != nil {
		libs.Fail(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}

func (h *Handlers) Commit(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

	var body struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
		Author  string `json:"author"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.Subject == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "subject is required")
		return
	}

	if err := h.git.Commit(reqCtx, wsID, body.Subject, body.Body, time.Now()); err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, wsID)
}

func (h *Handlers) Reset(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

	var body struct {
		Mode string `json:"mode"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.Mode == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "mode is required")
		return
	}

	if err := h.git.Reset(reqCtx, wsID, body.Mode, "", time.Now()); err != nil {
		libs.Fail(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}
