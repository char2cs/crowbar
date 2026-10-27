package handlers

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// StashPush POST /v0/chats/:chatId/git/stash
func (h *Handlers) StashPush(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

	var body struct {
		Message string `json:"message"`
	}
	if !bindOptional(ctx, &body) {
		return
	}

	if err := h.git.StashPush(reqCtx, wsID, body.Message, time.Now()); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	ctx.Status(http.StatusOK)
}

// StashApply POST /v0/chats/:chatId/git/stash-apply
func (h *Handlers) StashApply(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

	var body struct {
		Index int `json:"index"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	stashID := strconv.Itoa(body.Index)
	if err := h.git.StashApply(reqCtx, wsID, stashID, time.Now()); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	ctx.Status(http.StatusOK)
}

// StashPop POST /v0/chats/:chatId/git/stash-pop
func (h *Handlers) StashPop(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

	var body struct {
		Index int `json:"index"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	stashID := strconv.Itoa(body.Index)
	if err := h.git.StashPop(reqCtx, wsID, stashID, time.Now()); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	ctx.Status(http.StatusOK)
}

// resolveStashIndex resolves the stash index from the already-bound body,
// falling back to the ?index= query param when the body omits it. It returns
// the resolved index, or a non-empty human-readable reason when the index is
// missing (empty string) or not an integer.
func resolveStashIndex(
	ctx *gin.Context,
	bodyIndex *int,
) (int, string) {
	if bodyIndex != nil {
		return *bodyIndex, ""
	}
	raw := ctx.Query("index")
	if raw == "" {
		return 0, "index is required"
	}
	idx, err := strconv.Atoi(raw)
	if err != nil {
		return 0, "index must be an integer"
	}
	return idx, ""
}

// StashDrop DELETE /v0/chats/:chatId/git/stash
func (h *Handlers) StashDrop(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

	var body struct {
		Index *int `json:"index"`
	}
	if !bindOptional(ctx, &body) {
		return
	}
	index, reason := resolveStashIndex(ctx, body.Index)
	if reason != "" {
		libs.WriteErr(ctx, http.StatusBadRequest, reason)
		return
	}

	stashID := strconv.Itoa(index)
	if err := h.git.StashDrop(reqCtx, wsID, stashID, time.Now()); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// bindOptional decodes an optional JSON body into body: an empty one keeps the
// defaults, one that does not decode is a 400.
func bindOptional(
	ctx *gin.Context,
	body any,
) bool {
	if err := ctx.ShouldBindJSON(body); err != nil && !errors.Is(err, io.EOF) {
		libs.WriteErr(ctx, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
