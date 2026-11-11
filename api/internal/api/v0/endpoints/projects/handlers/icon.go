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

// A project icon is an uploaded image, an emoji, or the sidebar's default glyph.
// Unlike a repo there is no GitHub-avatar variant (a project has no origin remote)
// and no generated letter tile, so "no icon" needs nothing stored.

// Icon handles GET /v0/projects/:projectId/icon, serving the on-disk icon bytes
// with the content-type sniffed from them. 404 when the project has no icon.
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

// PutIcon handles PUT /v0/projects/:projectId/icon from a multipart "icon" field
// (web) or a JSON {"path"} the daemon reads (desktop: the WKWebView crowbar://
// scheme cannot carry a multipart body).
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
	// An image replaces an emoji, and new bytes behind the stable icon URL need
	// the version bumped so the DTO's ?v= param changes and clients refetch.
	hasIcon, emoji := true, ""
	h.saveIcon(c, projectID, project.Update{
		AvatarHasIcon:     &hasIcon,
		AvatarEmoji:       &emoji,
		BumpAvatarVersion: true,
	})
}

// PutIconEmoji handles PUT /v0/projects/:projectId/icon/emoji.
// Body: {"emoji":"🦊"}.
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
	// An emoji replaces an image: clear the flag and best-effort remove the file
	// behind it, so a later "reset" has nothing stale to fall back to.
	if path, ok := h.iconPath(c); ok {
		icons.Remove(c.Request.Context(), path)
	}
	hasIcon := false
	h.saveIcon(c, projectID, project.Update{
		AvatarEmoji:   &body.Emoji,
		AvatarHasIcon: &hasIcon,
	})
}

// DeleteIcon handles DELETE /v0/projects/:projectId/icon. It removes the
// on-disk icon and clears the emoji, resetting the project to the sidebar's
// default glyph.
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

// saveIcon persists an icon change and answers 204, like every icon mutation:
// the updated project arrives on the Projects stream, not in this response.
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

// iconPath resolves the project's icon file from the :projectId param and the
// configured crowbar home. ok is false when the home cannot be resolved.
func (h *Handlers) iconPath(
	c *gin.Context,
) (string, bool) {
	home, err := h.crowbarHome()
	if err != nil || home == "" {
		return "", false
	}
	return projectIconPath(home, c.Param("projectId")), true
}

// projectIconPath is <crowbarHome>/projects/<projectId>/icon, beside the repo
// directories; repo ids are uuids, so "icon" can never collide with one.
func projectIconPath(
	crowbarHome string,
	projectID string,
) string {
	return filepath.Join(crowbarHome, "projects", projectID, "icon")
}
