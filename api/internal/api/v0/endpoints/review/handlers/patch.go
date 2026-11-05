package handlers

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// defaultPatchMaxLines caps a patch request that did not ask for one: no file a human reads is
// cut, but a pathological generated file cannot be pulled into a browser by accident.
const defaultPatchMaxLines = 20000

// truncatedHeader reports that a patch was cut short. A leading header means holding the body
// until the last hunk, affordable only when a cap bounds it; uncapped patches stream with no
// header (fetch() exposes no HTTP trailers, so a trailer is not an option).
const truncatedHeader = "X-Crowbar-Diff-Truncated"

// maxBufferedPatchLines ceilings the client-supplied cap so it cannot buy an unbounded buffer.
// Larger caps take the streaming path and get no truncation header.
const maxBufferedPatchLines = 50000

// GetPatch handles GET /v0/chats/:chatId/review/patch?path=&maxLines=, streaming ONE file's
// unified patch as text/plain; maxLines <= 0 means unlimited. An empty path is refused because
// ":(top,literal)" alone matches the whole tree and would read the entire branch diff.
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

func (h *Handlers) streamPatch(
	ctx *gin.Context,
	path string,
	maxLines int,
) {
	if maxLines > 0 && maxLines <= maxBufferedPatchLines {
		h.bufferedPatch(ctx, path, maxLines)
		return
	}
	ctx.Header("Content-Type", "text/plain; charset=utf-8")
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

// bufferedPatch serves a capped request, holding the body until it knows whether it was cut so
// the answer travels in a header. The hold is bounded by maxLines <= maxBufferedPatchLines.
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
	ctx.Header("Content-Type", "text/plain; charset=utf-8")
	if truncated {
		ctx.Header(truncatedHeader, "true")
	}
	ctx.Status(http.StatusOK)
	if _, err := ctx.Writer.Write(buf.Bytes()); err != nil {
		libs.NoteError(ctx, err)
	}
}

// failPatch reports a streaming failure as best the response still allows: before the first byte
// it serves the JSON error envelope; after it the 200 is committed, so the error is only noted
// for the request log and the stream stops.
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

// patchMaxLines reads the cap: absent takes the default, non-numeric is a 400 rather than a
// silent fallback, and zero or negative pass through as the engine's "unlimited".
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
