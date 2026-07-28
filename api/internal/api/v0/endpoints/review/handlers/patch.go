package handlers

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// defaultPatchMaxLines caps a patch request that did not ask for one. It is
// generous enough that no file a human reads is ever cut, and small enough that
// the perf fixture's 420,000-line monster cannot be pulled into a browser by
// accident.
const defaultPatchMaxLines = 20000

// truncatedHeader names the field reporting that a patch was cut short.
//
// Whether the cap was reached is only known once the last hunk is written, so
// reporting it in a LEADING header means holding the body until then. That is
// affordable exactly when a cap was asked for, because the body is then bounded
// by the cap — which is the entire point of asking for one. The uncapped path,
// where holding the body would be the O(diff) behaviour this endpoint exists to
// abolish, streams instead and reports nothing, because an uncapped patch is
// uncuttable and there is nothing to report.
//
// An earlier revision used an HTTP trailer to keep both properties at once.
// That is correct HTTP and useless in practice: `fetch()` exposes no trailers,
// so the one client this API has could never read it.
const truncatedHeader = "X-Crowbar-Diff-Truncated"

// maxBufferedPatchLines bounds what a cap is allowed to make this handler hold.
// maxLines is client-supplied, so without a ceiling a request for ten million
// lines would buy a ten-million-line buffer under the guise of a limit. Beyond
// this the request is served by the streaming path: a caller asking for more
// lines than any view renders has opted out of the protection the cap exists to
// provide, and gets no truncation header.
const maxBufferedPatchLines = 50000

// GetPatch handles GET /v0/workspaces/:wsId/review/patch?path=&maxLines=,
// serving ONE file's unified patch as text/plain.
//
// The patch is streamed: git's output reaches the response as it is produced,
// so a 420,000-line file costs neither the daemon nor this handler an
// allocation proportional to it. maxLines defaults to defaultPatchMaxLines and
// <= 0 means unlimited — which also means uncuttable, so no truncation is
// promised or reported on that path.
//
// path is required. An empty one is refused here as well as inside the engine
// because ":(top,literal)" with nothing appended is a valid pathspec matching
// the whole tree: the one query guaranteed to be O(one file) would quietly
// become a read of the entire branch diff.
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
		ctx.Param("wsId"),
		scopeCommit(ctx),
		path,
		maxLines,
		ctx.Writer,
	)
	if err != nil {
		failPatch(ctx, err)
	}
}

// bufferedPatch serves a capped request, holding the body only long enough to
// learn whether it was cut so the answer can travel in a readable header. The
// hold is bounded by maxLines, which the caller chose and maxBufferedPatchLines
// ceilings; nothing here is proportional to the diff.
func (h *Handlers) bufferedPatch(
	ctx *gin.Context,
	path string,
	maxLines int,
) {
	var buf bytes.Buffer
	_, truncated, err := h.reviewUsecase.GetPatch(
		ctx.Request.Context(),
		ctx.Param("wsId"),
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
	_, _ = ctx.Writer.Write(buf.Bytes())
}

// failPatch reports a streaming failure as best the response still allows.
//
// Before the first byte nothing is committed, so the reserved text/plain is
// withdrawn and the usual JSON error envelope is served in its place. After it, the status line is already 200 and the
// client already holds part of a patch: the error is recorded on the context
// for the request log and the stream simply stops, because rewriting a response
// that is half delivered is not something HTTP offers.
func failPatch(
	ctx *gin.Context,
	err error,
) {
	if ctx.Writer.Written() {
		_ = ctx.Error(err)
		return
	}
	ctx.Writer.Header().Del("Content-Type")
	libs.WriteErr(ctx, reviewErrorStatus(err), err.Error())
}

// patchMaxLines reads the cap. An absent value takes the default; a
// non-numeric one is a 400 rather than a silent fallback, because a client that
// mistyped the cap would otherwise be quietly served a different amount of
// patch than it asked for. Zero and negative values pass through untouched:
// they are the engine's "unlimited".
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
