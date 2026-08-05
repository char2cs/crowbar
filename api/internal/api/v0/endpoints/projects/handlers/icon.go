package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/icons"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
)

// The project icon, on the same mechanism a repo's already used: an uploaded
// image, an emoji, or the sidebar's default.
//
// What a project does NOT get is the repo's `PUT .../icon/github`. That one
// reads the repo's `origin` remote to find an owner avatar, and a project is a
// folder holding repos — it has no remote of its own, and the repos under it may
// have several between them. Offering it would be a button that could only ever
// fail.
//
// The default differs too, and that is the reason there is no avatarLabel or
// avatarColor here. A repo with no icon falls back to a generated letter tile; a
// project with no icon falls back to the Library glyph the sidebar draws for
// every project, so "no icon" is a state the client already knows how to render
// and the daemon has nothing to store for it.

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

// PutIcon handles PUT /v0/projects/:projectId/icon. It accepts the icon two
// ways: a multipart/form-data "icon" field (web browsers), or a JSON body
// {"path": "<absolute path>"} the daemon reads from disk itself. The latter is
// the desktop path — the WKWebView crowbar:// scheme cannot carry a
// multipart/binary request body, so the native file dialog yields a path and the
// daemon reads it, the same way repo import reads a user-selected path.
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
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
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
		_ = os.Remove(path)
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
		_ = os.Remove(path)
	}
	hasIcon, emoji := false, ""
	h.saveIcon(c, projectID, project.Update{AvatarHasIcon: &hasIcon, AvatarEmoji: &emoji})
}

// saveIcon persists an icon change and delivers the updated project on the
// Projects WS stream. 204, consistent with every other icon mutation: the FE
// apiFetch throws on any non-enveloped 200 body, and the new avatar arrives on
// the stream rather than in this response.
func (h *Handlers) saveIcon(
	c *gin.Context,
	projectID string,
	in project.Update,
) {
	updated, err := h.reader.Update(c.Request.Context(), projectID, in)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	h.broadcast(dto.ProjectDTOFrom(updated))
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

// projectIconPath is <crowbarHome>/projects/<projectId>/icon.
//
// It sits BESIDE the repo directories rather than inside one of them: a repo's
// own icon is <crowbarHome>/projects/<projectId>/<repoId>/icon, and repo ids are
// uuids, so a file plainly named "icon" at the project level can never collide
// with a repo's directory.
func projectIconPath(
	crowbarHome string,
	projectID string,
) string {
	return filepath.Join(crowbarHome, "projects", projectID, "icon")
}
