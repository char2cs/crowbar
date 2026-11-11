package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// CreateBranch POST /v0/chats/:chatId/git/branches
func (h *Handlers) CreateBranch(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

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
		libs.Fail(ctx, err)
		return
	}

	ctx.Status(http.StatusCreated)
}

// RenameBranch PATCH /v0/chats/:chatId/git/branches
func (h *Handlers) RenameBranch(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

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
		libs.Fail(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}

// DeleteBranch DELETE /v0/chats/:chatId/git/branches
func (h *Handlers) DeleteBranch(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

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
		libs.Fail(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// Switch POST /v0/chats/:chatId/git/switch
func (h *Handlers) Switch(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

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
		libs.Fail(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}
