package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

type receivedDTO struct {
	Received int64 `json:"received"`
}

func (h *Handlers) BeginAttachmentUpload(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	var body struct {
		ID       string `json:"id" binding:"required"`
		FileName string `json:"fileName" binding:"required"`
		Size     int64  `json:"size"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, "id and fileName required")
		return
	}
	received, err := h.turns.BeginAttachmentUpload(ctx.Request.Context(), chat.ID, body.ID, body.FileName, body.Size)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, receivedDTO{Received: received})
}

func (h *Handlers) AppendAttachmentUpload(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	offset, err := strconv.ParseInt(ctx.Query("offset"), 10, 64)
	if err != nil || offset < 0 {
		libs.WriteErr(ctx, http.StatusBadRequest, "offset required")
		return
	}
	received, err := h.turns.AppendAttachmentUpload(
		ctx.Request.Context(), chat.ID, ctx.Param("uploadId"), offset, ctx.Request.Body)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, receivedDTO{Received: received})
}

func (h *Handlers) CompleteAttachmentUpload(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	var body struct {
		FileName string `json:"fileName" binding:"required"`
		Size     int64  `json:"size"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, "fileName required")
		return
	}
	stored, err := h.turns.CompleteAttachmentUpload(
		ctx.Request.Context(), chat.ID, ctx.Param("uploadId"), body.FileName, body.Size)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryWithStatus(ctx, http.StatusCreated, dto.ChatAttachmentDTO{
		Ref: stored.Ref, FileName: stored.FileName, Size: stored.Size, ContentType: stored.ContentType,
	})
}

func (h *Handlers) AbortAttachmentUpload(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	if err := h.turns.AbortAttachmentUpload(ctx.Request.Context(), chat.ID, ctx.Param("uploadId")); err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteNoContent(ctx)
}
