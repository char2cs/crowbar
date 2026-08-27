package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

// GetDefaultPermissionLevel handles GET /v0/settings/chat/permission-level.
func (h *Handlers) GetDefaultPermissionLevel(
	ctx *gin.Context,
) {
	level, err := h.providers.DefaultPermissionLevel(ctx.Request.Context())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, gin.H{"level": string(level)})
}

// PutDefaultPermissionLevel handles PUT /v0/settings/chat/permission-level.
func (h *Handlers) PutDefaultPermissionLevel(
	ctx *gin.Context,
) {
	var body struct {
		Level string `json:"level"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	err := h.providers.SetDefaultPermissionLevel(ctx.Request.Context(), agentusecase.PermissionLevel(body.Level))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, gin.H{"level": body.Level})
}
