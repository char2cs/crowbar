package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/api/libs"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ListProfiles handles GET /v0/settings/terminal/profiles.
func (h *Handlers) ListProfiles(ctx *gin.Context) {
	profiles, err := h.profileStore.FindAll(ctx.Request.Context())
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	libs.WriteQueryOK(ctx, profiles)
}

// GetProfile handles GET /v0/settings/terminal/profiles/:id.
func (h *Handlers) GetProfile(ctx *gin.Context) {
	id := ctx.Param("id")
	p, err := h.profileStore.FindByKey(ctx.Request.Context(), id)
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if p == nil {
		libs.WriteErr(ctx, http.StatusNotFound, "profile not found")
		return
	}

	libs.WriteQueryOK(ctx, p)
}

// CreateProfile handles POST /v0/settings/terminal/profiles.
func (h *Handlers) CreateProfile(ctx *gin.Context) {
	var p domain.TerminalProfile
	if err := ctx.ShouldBindJSON(&p); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if p.ID == "" {
		p.ID = uuid.NewString()
	}

	if err := h.profileStore.Save(ctx.Request.Context(), p); err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	libs.WriteQueryWithStatus(ctx, http.StatusCreated, p)
}

// UpdateProfile handles PUT /v0/settings/terminal/profiles/:id.
func (h *Handlers) UpdateProfile(ctx *gin.Context) {
	id := ctx.Param("id")

	existing, err := h.profileStore.FindByKey(ctx.Request.Context(), id)
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		libs.WriteErr(ctx, http.StatusNotFound, "profile not found")
		return
	}

	var p domain.TerminalProfile
	if err := ctx.ShouldBindJSON(&p); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	p.ID = id

	if err := h.profileStore.Save(ctx.Request.Context(), p); err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	libs.WriteQueryOK(ctx, p)
}

// DeleteProfile handles DELETE /v0/settings/terminal/profiles/:id.
func (h *Handlers) DeleteProfile(ctx *gin.Context) {
	id := ctx.Param("id")

	existing, err := h.profileStore.FindByKey(ctx.Request.Context(), id)
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		libs.WriteErr(ctx, http.StatusNotFound, "profile not found")
		return
	}

	if err := h.profileStore.Delete(ctx.Request.Context(), id); err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	ctx.Status(http.StatusNoContent)
}
