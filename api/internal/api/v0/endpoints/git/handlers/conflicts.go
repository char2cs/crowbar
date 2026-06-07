package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// resolveRequest is the POST conflicts/resolve body: the conflicting file path,
// the ConflictHunk id to resolve, the chosen resolution, and the custom resolved
// content used only when resolution is "custom".
type resolveRequest struct {
	Path            string `json:"path"`
	ConflictHunkID  string `json:"conflictHunkId"`
	Resolution      string `json:"resolution"`
	ResolvedContent string `json:"resolvedContent"`
}

func (r resolveRequest) resolution() gitdomain.ConflictResolution {
	return gitdomain.ConflictResolution(r.Resolution)
}

func validResolution(
	resolution gitdomain.ConflictResolution,
) bool {
	switch resolution {
	case gitdomain.ConflictResolutionOurs,
		gitdomain.ConflictResolutionTheirs,
		gitdomain.ConflictResolutionBoth,
		gitdomain.ConflictResolutionCustom,
		gitdomain.ConflictResolutionUnresolved:
		return true
	default:
		return false
	}
}

func (r resolveRequest) validate() string {
	if r.Path == "" {
		return "path is required"
	}
	if r.ConflictHunkID == "" {
		return "conflictHunkId is required"
	}
	if !validResolution(r.resolution()) {
		return "resolution must be one of ours, theirs, both, custom, unresolved"
	}
	if r.resolution() == gitdomain.ConflictResolutionCustom && r.ResolvedContent == "" {
		return "resolvedContent is required when resolution is custom"
	}
	return ""
}

// Conflicts handles GET /v0/workspaces/:wsId/git/conflicts, returning each
// conflicting file together with its three-way ConflictHunks as a
// ConflictFileDTO list.
func (h *Handlers) Conflicts(
	c *gin.Context,
) {
	files, err := h.conflicts(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		code, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, code, msg)
		return
	}
	libs.WriteQueryOK(c, files)
}

func (h *Handlers) conflicts(
	ctx context.Context,
	wsID string,
) ([]dto.ConflictFileDTO, error) {
	paths, err := h.git.ConflictedFiles(ctx, wsID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ConflictFileDTO, 0, len(paths))
	for _, path := range paths {
		hunks, hErr := h.git.ConflictHunks(ctx, wsID, path)
		if hErr != nil {
			return nil, hErr
		}
		out = append(out, dto.ConflictFileDTOFrom(path, hunks))
	}
	return out, nil
}

// ResolveConflict handles POST /v0/workspaces/:wsId/git/conflicts/resolve,
// applying the chosen resolution to the addressed conflict hunk. The body
// requires path, conflictHunkId, and a valid resolution; resolvedContent is
// required only when resolution is custom. The response echoes the workspace id.
func (h *Handlers) ResolveConflict(
	c *gin.Context,
) {
	var body resolveRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if msg := body.validate(); msg != "" {
		libs.WriteErr(c, http.StatusBadRequest, msg)
		return
	}
	err := h.git.ResolveHunk(
		c.Request.Context(),
		c.Param("wsId"),
		body.Path,
		body.ConflictHunkID,
		body.resolution(),
		body.ResolvedContent,
		h.now(),
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}
