package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
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

// Stage POST /workspaces/:wsId/git/stage
func (h *Handlers) Stage(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	paths, ok := bindPaths(ctx)
	if !ok {
		return
	}

	now := time.Now()
	for _, p := range paths {
		if err := h.git.StageFile(reqCtx, wsID, p, now); err != nil {
			status, msg := libs.StatusAndMessage(err)
			libs.WriteErr(ctx, status, msg)
			return
		}
	}

	libs.WriteMutationOK(ctx, http.StatusOK, wsID)
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

	if err := h.git.StageHunk(reqCtx, wsID, body.Path, body.HunkID, time.Now()); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
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

	paths, ok := bindPaths(ctx)
	if !ok {
		return
	}

	now := time.Now()
	for _, p := range paths {
		if err := h.git.UnstageFile(reqCtx, wsID, p, now); err != nil {
			status, msg := libs.StatusAndMessage(err)
			libs.WriteErr(ctx, status, msg)
			return
		}
	}

	libs.WriteMutationOK(ctx, http.StatusOK, wsID)
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

	if err := h.git.UnstageHunk(reqCtx, wsID, body.Path, body.HunkID, time.Now()); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
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

	paths, ok := bindPaths(ctx)
	if !ok {
		return
	}

	now := time.Now()
	for _, p := range paths {
		if err := h.git.Discard(reqCtx, wsID, p, now); err != nil {
			status, msg := libs.StatusAndMessage(err)
			libs.WriteErr(ctx, status, msg)
			return
		}
	}

	libs.WriteMutationOK(ctx, http.StatusOK, wsID)
}

// Commit POST /workspaces/:wsId/git/commit
func (h *Handlers) Commit(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, wsID)
}

// Push POST /workspaces/:wsId/git/push is a slow git op: it returns 202 and runs
// the push in the background, leaving the post-push state to the git-status
// watcher broadcast and a failure to the workspace LastError (00 §4).
func (h *Handlers) Push(
	ctx *gin.Context,
) {
	wsID := ctx.Param("wsId")

	libs.WriteAccepted(ctx)
	h.runAsync(
		ctx.Request.Context(),
		wsID,
		func(c context.Context) error {
			return h.git.Push(c, wsID, time.Now())
		},
	)
}

// Fetch POST /workspaces/:wsId/git/fetch is a slow git op (202 + async).
func (h *Handlers) Fetch(
	ctx *gin.Context,
) {
	wsID := ctx.Param("wsId")

	libs.WriteAccepted(ctx)
	h.runAsync(
		ctx.Request.Context(),
		wsID,
		func(c context.Context) error {
			return h.git.Fetch(c, wsID, time.Now())
		},
	)
}

// Pull POST /workspaces/:wsId/git/pull is a slow git op (202 + async).
func (h *Handlers) Pull(
	ctx *gin.Context,
) {
	wsID := ctx.Param("wsId")

	libs.WriteAccepted(ctx)
	h.runAsync(
		ctx.Request.Context(),
		wsID,
		func(c context.Context) error {
			return h.git.Pull(c, wsID, "", time.Now())
		},
	)
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
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "name is required")
		return
	}

	if err := h.git.CreateBranch(reqCtx, wsID, body.Name, "", false, time.Now()); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
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
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.From == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "from is required")
		return
	}
	if body.To == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "to is required")
		return
	}

	if err := h.git.RenameBranch(reqCtx, wsID, body.From, body.To, time.Now()); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
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
		libs.WriteErr(ctx, http.StatusBadRequest, "name is required")
		return
	}

	if err := h.git.DeleteBranch(reqCtx, wsID, body.Name, time.Now()); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
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
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.Branch == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "branch is required")
		return
	}

	if err := h.git.SwitchBranch(reqCtx, wsID, body.Branch, time.Now()); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
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

// StashDrop DELETE /workspaces/:wsId/git/stash
func (h *Handlers) StashDrop(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Index *int `json:"index"`
	}
	_ = ctx.ShouldBindJSON(&body)
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
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.Mode == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "mode is required")
		return
	}

	if err := h.git.Reset(reqCtx, wsID, body.Mode, "", time.Now()); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	ctx.Status(http.StatusOK)
}

// Merge POST /workspaces/:wsId/git/merge is a slow git op: it validates the
// target branch synchronously (4xx) then returns 202 and runs the merge in the
// background (00 §4).
func (h *Handlers) Merge(
	ctx *gin.Context,
) {
	wsID := ctx.Param("wsId")

	var body struct {
		Branch string `json:"branch"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.Branch == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "branch is required")
		return
	}

	libs.WriteAccepted(ctx)
	h.runAsync(
		ctx.Request.Context(),
		wsID,
		func(c context.Context) error {
			return h.git.Merge(c, wsID, body.Branch, time.Now())
		},
	)
}

// Rebase POST /workspaces/:wsId/git/rebase is a slow git op: it validates the
// onto branch synchronously (4xx) then returns 202 and runs the rebase in the
// background (00 §4).
func (h *Handlers) Rebase(
	ctx *gin.Context,
) {
	wsID := ctx.Param("wsId")

	var body struct {
		Branch string `json:"branch"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.Branch == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "branch is required")
		return
	}

	libs.WriteAccepted(ctx)
	h.runAsync(
		ctx.Request.Context(),
		wsID,
		func(c context.Context) error {
			return h.git.Rebase(c, wsID, body.Branch, time.Now())
		},
	)
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
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.Path == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path is required")
		return
	}
	if body.Choice == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "choice is required")
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	ctx.Status(http.StatusOK)
}
