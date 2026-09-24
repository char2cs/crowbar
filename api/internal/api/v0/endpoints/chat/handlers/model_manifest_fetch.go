package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// GetModelManifestFetchEnabled handles GET /v0/settings/chat/model-manifest-fetch.
func (h *Handlers) GetModelManifestFetchEnabled(
	ctx *gin.Context,
) {
	enabled, err := h.providers.ModelManifestFetchEnabled(ctx.Request.Context())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, gin.H{"enabled": enabled})
}

// PutModelManifestFetchEnabled handles PUT /v0/settings/chat/model-manifest-fetch.
func (h *Handlers) PutModelManifestFetchEnabled(
	ctx *gin.Context,
) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	err := h.providers.SetModelManifestFetchEnabled(ctx.Request.Context(), body.Enabled)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, gin.H{"enabled": body.Enabled})
}
