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

// Icon handles GET /v0/projects/:projectId/repos/:repoId/icon. It serves the
// on-disk icon bytes stored at worktreepath.RepoIconPath, sniffing the
// content-type from the bytes. Returns 404 when the repo has no on-disk icon.
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

// iconPath resolves the entity-scoped icon file path from the request's
// :projectId/:repoId params and the configured crowbar home. ok is false when
// the home cannot be resolved.
func (h *Handlers) iconPath(
	c *gin.Context,
) (string, bool) {
	home, err := h.crowbarHome()
	if err != nil || home == "" {
		return "", false
	}
	return repoIconPath(home, c.Param("projectId"), c.Param("repoId")), true
}

// repoIconPath mirrors worktreepath.RepoIconPath without importing the
// usecase-internal package (forbidden from the api layer):
// <crowbarHome>/projects/<projectId>/<repoId>/icon.
func repoIconPath(
	crowbarHome string,
	projectID string,
	repoID string,
) string {
	return filepath.Join(crowbarHome, "projects", projectID, repoID, "icon")
}

// PutIconEmoji handles PUT /v0/projects/:projectId/repos/:repoId/icon/emoji.
// Body: {"emoji":"🦊"} — stores "emoji:🦊" in avatar_url.
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
	// Emoji takes precedence over an on-disk image: clear the icon flag and
	// best-effort remove the stored icon file.
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

// DeleteIcon handles DELETE /v0/projects/:projectId/repos/:repoId/icon.
// Removes the on-disk icon file and clears both the icon flag and the emoji,
// resetting the repo to its generated label/color avatar.
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

// PutIcon handles PUT /v0/projects/:projectId/repos/:repoId/icon from a multipart "icon" field or
// a JSON {"path"} the daemon reads itself (the desktop crowbar:// scheme cannot carry a multipart
// body). Accepts png, jpeg, webp up to 5 MB.
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
	// New bytes behind the stable icon URL: bump the version so the DTO's
	// ?v= param changes and clients refetch the image.
	repo.AvatarVersion++
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.Fail(c, err)
		return
	}
	// 204, consistent with the other icon mutations: the FE apiFetch throws on
	// any non-enveloped 200 body, and the store's save publishes the avatar.
	c.Status(http.StatusNoContent)
}

// storeIconBytes writes raw icon bytes to this repo's entity-scoped icon path.
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

// PutIconGithub handles PUT /v0/projects/:projectId/repos/:repoId/icon/github.
// Downloads the repo owner's GitHub avatar bytes and stores them at the
// entity-scoped icon path, setting AvatarHasIcon.
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
	// New bytes behind the stable icon URL: bump the version so the DTO's
	// ?v= param changes and clients refetch the image.
	repo.AvatarVersion++
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
