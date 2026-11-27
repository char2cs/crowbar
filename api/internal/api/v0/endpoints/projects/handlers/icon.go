package handlers

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/icons"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
)

func (h *Handlers) Icon(
	c *gin.Context,
) {
	proj, err := h.reader.Get(c.Request.Context(), c.Param("projectId"))
	if err != nil || !proj.AvatarHasIcon {
		c.Status(http.StatusNotFound)
		return
	}
	path, ok := h.iconPath(c)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	icons.Serve(c, path)
}

func (h *Handlers) PutIcon(
	c *gin.Context,
) {
	projectID := c.Param("projectId")
	if _, err := h.reader.Get(c.Request.Context(), projectID); err != nil {
		libs.WriteErr(c, http.StatusNotFound, "project not found")
		return
	}
	data, ok := icons.ReadUpload(c)
	if !ok {
		return
	}
	if !icons.Validate(c, data) {
		return
	}
	path, ok := h.iconPath(c)
	if !ok {
		libs.WriteErr(c, http.StatusInternalServerError, "could not resolve icon path")
		return
	}
	if err := icons.Store(path, data); err != nil {
		libs.Fail(c, err)
		return
	}
	hasIcon, emoji := true, ""
	h.saveIcon(c, projectID, project.Update{
		AvatarHasIcon:     &hasIcon,
		AvatarEmoji:       &emoji,
		BumpAvatarVersion: true,
	})
}

func (h *Handlers) PutIconEmoji(
	c *gin.Context,
) {
	projectID := c.Param("projectId")
	if _, err := h.reader.Get(c.Request.Context(), projectID); err != nil {
		libs.WriteErr(c, http.StatusNotFound, "project not found")
		return
	}
	var body struct {
		Emoji string `json:"emoji"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Emoji == "" {
		libs.WriteErr(c, http.StatusBadRequest, "emoji required")
		return
	}
	body.Emoji = strings.TrimSpace(body.Emoji)
	if !icons.IsSingleEmoji(body.Emoji) {
		libs.WriteErr(c, http.StatusBadRequest, "emoji must be a single character")
		return
	}
	if path, ok := h.iconPath(c); ok {
		icons.Remove(c.Request.Context(), path)
	}
	hasIcon := false
	h.saveIcon(c, projectID, project.Update{
		AvatarEmoji:   &body.Emoji,
		AvatarHasIcon: &hasIcon,
	})
}

func (h *Handlers) DeleteIcon(
	c *gin.Context,
) {
	projectID := c.Param("projectId")
	if _, err := h.reader.Get(c.Request.Context(), projectID); err != nil {
		libs.WriteErr(c, http.StatusNotFound, "project not found")
		return
	}
	if path, ok := h.iconPath(c); ok {
		icons.Remove(c.Request.Context(), path)
	}
	hasIcon, emoji := false, ""
	h.saveIcon(c, projectID, project.Update{AvatarHasIcon: &hasIcon, AvatarEmoji: &emoji})
}

func (h *Handlers) saveIcon(
	c *gin.Context,
	projectID string,
	in project.Update,
) {
	if _, err := h.reader.Update(c.Request.Context(), projectID, in); err != nil {
		libs.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handlers) iconPath(
	c *gin.Context,
) (string, bool) {
	home, err := h.crowbarHome()
	if err != nil || home == "" {
		return "", false
	}
	return projectIconPath(home, c.Param("projectId")), true
}

func projectIconPath(
	crowbarHome string,
	projectID string,
) string {
	return filepath.Join(crowbarHome, "projects", projectID, "icon")
}
