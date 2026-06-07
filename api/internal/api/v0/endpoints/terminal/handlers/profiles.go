package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ListProfiles handles GET /v0/settings/terminal/profiles. It returns every
// stored profile under data as a non-nil list.
func (h *Handlers) ListProfiles(
	c *gin.Context,
) {
	profiles, err := h.profiles.FindAll(c.Request.Context())
	if err != nil {
		libs.WriteErr(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	libs.WriteQueryOK(c, dto.TerminalProfileDTOList(profiles))
}

// GetProfile handles GET /v0/settings/terminal/profiles/:id. It returns the
// stored profile under data, or 404 when the id is unknown.
func (h *Handlers) GetProfile(
	c *gin.Context,
) {
	prof, err := h.profiles.FindByKey(c.Request.Context(), c.Param("id"))
	if err != nil {
		libs.WriteErr(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}
	if prof == nil {
		libs.WriteErr(
			c,
			http.StatusNotFound,
			"profile not found",
		)
		return
	}

	libs.WriteQueryOK(c, dto.TerminalProfileDTOFrom(*prof))
}

// CreateProfile handles POST /v0/settings/terminal/profiles. It assigns an id
// when the body omits one, persists the profile, and returns it under data.
func (h *Handlers) CreateProfile(
	c *gin.Context,
) {
	var prof domain.TerminalProfile
	if err := c.ShouldBindJSON(&prof); err != nil {
		libs.WriteErr(
			c,
			http.StatusBadRequest,
			err.Error(),
		)
		return
	}

	if prof.ID == "" {
		prof.ID = uuid.NewString()
	}

	if err := h.profiles.Save(c.Request.Context(), prof); err != nil {
		libs.WriteErr(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	libs.WriteQueryWithStatus(
		c,
		http.StatusCreated,
		dto.TerminalProfileDTOFrom(prof),
	)
}

// UpdateProfile handles PUT /v0/settings/terminal/profiles/:id. It rejects an
// unknown id with 404, then persists the body under that id and returns it.
func (h *Handlers) UpdateProfile(
	c *gin.Context,
) {
	id := c.Param("id")
	existing, err := h.profiles.FindByKey(c.Request.Context(), id)
	if err != nil {
		libs.WriteErr(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}
	if existing == nil {
		libs.WriteErr(
			c,
			http.StatusNotFound,
			"profile not found",
		)
		return
	}

	var prof domain.TerminalProfile
	if bindErr := c.ShouldBindJSON(&prof); bindErr != nil {
		libs.WriteErr(
			c,
			http.StatusBadRequest,
			bindErr.Error(),
		)
		return
	}
	prof.ID = id

	if saveErr := h.profiles.Save(c.Request.Context(), prof); saveErr != nil {
		libs.WriteErr(
			c,
			http.StatusInternalServerError,
			saveErr.Error(),
		)
		return
	}

	libs.WriteQueryOK(c, dto.TerminalProfileDTOFrom(prof))
}

// DeleteProfile handles DELETE /v0/settings/terminal/profiles/:id. It rejects an
// unknown id with 404, then removes the profile and returns 204.
func (h *Handlers) DeleteProfile(
	c *gin.Context,
) {
	id := c.Param("id")
	existing, err := h.profiles.FindByKey(c.Request.Context(), id)
	if err != nil {
		libs.WriteErr(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}
	if existing == nil {
		libs.WriteErr(
			c,
			http.StatusNotFound,
			"profile not found",
		)
		return
	}

	if delErr := h.profiles.Delete(c.Request.Context(), id); delErr != nil {
		libs.WriteErr(
			c,
			http.StatusInternalServerError,
			delErr.Error(),
		)
		return
	}

	c.Status(http.StatusNoContent)
}
