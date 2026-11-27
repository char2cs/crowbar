package handlers

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

const defaultPatchMaxLines = 20000

const truncatedHeader = "X-Crowbar-Diff-Truncated"

const maxBufferedPatchLines = 50000

func (h *Handlers) GetPatch(
	ctx *gin.Context,
) {
	path := ctx.Query("path")
	if path == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path query parameter is required")
		return
	}
	maxLines, err := patchMaxLines(ctx.Query("maxLines"))
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.streamPatch(ctx, path, maxLines)
}

const patchContentType = "text/plain; charset=utf-8"

func (h *Handlers) streamPatch(
	ctx *gin.Context,
	path string,
	maxLines int,
) {
	if maxLines > 0 && maxLines <= maxBufferedPatchLines {
		h.bufferedPatch(ctx, path, maxLines)
		return
	}
	ctx.Header("Content-Type", patchContentType)
	libs.SetUntrustedFileHeaders(ctx, patchContentType)
	ctx.Status(http.StatusOK)
	_, _, err := h.reviewUsecase.GetPatch(
		ctx.Request.Context(),
		h.workspaceID(ctx),
		scopeCommit(ctx),
		path,
		maxLines,
		ctx.Writer,
	)
	if err != nil {
		failPatch(ctx, err)
	}
}

func (h *Handlers) bufferedPatch(
	ctx *gin.Context,
	path string,
	maxLines int,
) {
	var buf bytes.Buffer
	_, truncated, err := h.reviewUsecase.GetPatch(
		ctx.Request.Context(),
		h.workspaceID(ctx),
		scopeCommit(ctx),
		path,
		maxLines,
		&buf,
	)
	if err != nil {
		failPatch(ctx, err)
		return
	}
	ctx.Header("Content-Type", patchContentType)
	libs.SetUntrustedFileHeaders(ctx, patchContentType)
	if truncated {
		ctx.Header(truncatedHeader, "true")
	}
	ctx.Status(http.StatusOK)
	if _, err := ctx.Writer.Write(buf.Bytes()); err != nil {
		libs.NoteError(ctx, err)
	}
}

func failPatch(
	ctx *gin.Context,
	err error,
) {
	if ctx.Writer.Written() {
		libs.NoteError(ctx, err)
		return
	}
	ctx.Writer.Header().Del("Content-Type")
	libs.Fail(ctx, err)
}

func patchMaxLines(
	raw string,
) (int, error) {
	if raw == "" {
		return defaultPatchMaxLines, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("maxLines query parameter must be an integer")
	}
	return n, nil
}
