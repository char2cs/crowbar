package handlers

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// branchOutRequest names the branch a thread made a branch gets; "" generates one.
type branchOutRequest struct {
	Branch string `json:"branch"`
}

// BranchOut handles POST .../chats/:id/branch-out: the thread becomes a branch cut from
// its folder's own HEAD at its turn boundary. 202: the move is recorded, not yet made.
func (h *Handlers) BranchOut(
	ctx *gin.Context,
) {
	var body branchOutRequest
	if err := ctx.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.folders.BranchOut(ctx.Request.Context(), ctx.Param("id"), strings.TrimSpace(body.Branch)); err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteMutationOK(ctx, http.StatusAccepted, ctx.Param("id"))
}

// CancelMove handles POST .../chats/:id/move/cancel: the chat stays where it runs.
func (h *Handlers) CancelMove(
	ctx *gin.Context,
) {
	if err := h.folders.CancelMove(ctx.Request.Context(), ctx.Param("id")); err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteMutationOK(ctx, http.StatusOK, ctx.Param("id"))
}

// answerMoveRequest is the person's word on a move the agent asked for.
type answerMoveRequest struct {
	RequestID string `json:"requestId" binding:"required"`
	Allow     bool   `json:"allow"`
	Reason    string `json:"reason"`
}

// AnswerMove handles POST .../chats/:id/move/answer: Allow runs the agent's move at the turn
// boundary, Deny withdraws it with the person's reason.
func (h *Handlers) AnswerMove(
	ctx *gin.Context,
) {
	var body answerMoveRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	err := h.folders.AnswerMove(ctx.Request.Context(), ctx.Param("id"), body.RequestID, body.Allow,
		strings.TrimSpace(body.Reason))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteMutationOK(ctx, http.StatusOK, ctx.Param("id"))
}
