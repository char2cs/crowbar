package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/icons"
)

func (h *Handlers) Icon(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil || !repo.AvatarHasIcon {
		c.Status(http.StatusNotFound)
		return
	}
	iconPath, ok := h.iconPath(c)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	icons.Serve(c, iconPath)
}

func (h *Handlers) iconPath(
	c *gin.Context,
) (string, bool) {
	home, err := h.crowbarHome()
	if err != nil || home == "" {
		return "", false
	}
	return repoIconPath(home, c.Param("projectId"), c.Param("repoId")), true
}

func repoIconPath(
	crowbarHome string,
	projectID string,
	repoID string,
) string {
	return filepath.Join(crowbarHome, "projects", projectID, repoID, "icon")
}

func (h *Handlers) PutIconEmoji(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
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
	if iconPath, ok := h.iconPath(c); ok {
		icons.Remove(c.Request.Context(), iconPath)
	}
	repo.AvatarEmoji = body.Emoji
	repo.AvatarHasIcon = false
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handlers) DeleteIcon(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	if iconPath, ok := h.iconPath(c); ok {
		icons.Remove(c.Request.Context(), iconPath)
	}
	repo.AvatarHasIcon = false
	repo.AvatarEmoji = ""
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handlers) PutIcon(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	data, ok := icons.ReadUpload(c)
	if !ok {
		return
	}
	if !icons.Validate(c, data) {
		return
	}
	if err := h.storeIconBytes(c, data); err != nil {
		libs.Fail(c, err)
		return
	}

	repo.AvatarHasIcon = true
	repo.AvatarEmoji = ""
	repo.AvatarVersion++
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handlers) storeIconBytes(
	c *gin.Context,
	data []byte,
) error {
	iconPath, ok := h.iconPath(c)
	if !ok {
		return fmt.Errorf("could not resolve icon path")
	}
	return icons.Store(iconPath, data)
}

func (h *Handlers) PutIconGithub(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	if repo.Path == "" {
		libs.WriteErr(c, http.StatusUnprocessableEntity, "repo has no local path")
		return
	}
	data, _, err := h.fetchAvatar(c.Request.Context(), repo.Path)
	if err != nil || len(data) == 0 {
		libs.WriteErr(c, http.StatusUnprocessableEntity, "could not fetch GitHub avatar")
		return
	}
	if err := h.storeIconBytes(c, data); err != nil {
		libs.Fail(c, err)
		return
	}
	repo.AvatarHasIcon = true
	repo.AvatarEmoji = ""
	repo.AvatarVersion++
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
