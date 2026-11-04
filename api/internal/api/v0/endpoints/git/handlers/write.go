package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// pathsBody is the request body shared by the stage/unstage/discard handlers:
// a list of repo-relative paths, where "." means "everything".
type pathsBody struct {
	Paths []string `json:"paths"`
}

// bindPaths binds and validates a pathsBody, writing the 400 response itself
// when the body is malformed or empty. The bool reports whether the paths are
// usable.
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

// Stage POST /v0/chats/:chatId/git/stage
func (h *Handlers) Stage(ctx *gin.Context) { h.eachPath(ctx, h.git.StageFile) }

// StageHunk POST /v0/chats/:chatId/git/stage-hunk
func (h *Handlers) StageHunk(ctx *gin.Context) { h.oneHunk(ctx, h.git.StageHunk) }

// Unstage POST /v0/chats/:chatId/git/unstage
func (h *Handlers) Unstage(ctx *gin.Context) { h.eachPath(ctx, h.git.UnstageFile) }

// UnstageHunk POST /v0/chats/:chatId/git/unstage-hunk
func (h *Handlers) UnstageHunk(ctx *gin.Context) { h.oneHunk(ctx, h.git.UnstageHunk) }

// Discard POST /v0/chats/:chatId/git/discard
func (h *Handlers) Discard(ctx *gin.Context) { h.eachPath(ctx, h.git.Discard) }

// eachPath applies op to every path of a pathsBody, stopping at the first failure.
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

// oneHunk applies op to the {path, hunkId} the body names.
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

// Commit POST /v0/chats/:chatId/git/commit
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

// Reset POST /v0/chats/:chatId/git/reset
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
