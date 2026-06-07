package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// stageRequest is the POST stage / unstage body. It accepts EITHER a list of
// workspace-relative paths (file-level staging) OR a single path plus a hunkId
// (hunk-level staging). The frontend never sends raw patch text; hunkId
// addresses a hunk the engine already knows.
type stageRequest struct {
	Paths  []string `json:"paths"`
	Path   string   `json:"path"`
	HunkID string   `json:"hunkId"`
}

// discardRequest is the POST discard body: the workspace-relative paths whose
// working-tree changes are reverted.
type discardRequest struct {
	Paths []string `json:"paths"`
}

func (r stageRequest) isHunk() bool {
	return r.HunkID != ""
}

func (r stageRequest) valid() bool {
	if r.isHunk() {
		return r.Path != ""
	}
	return len(r.Paths) > 0
}

// Stage handles POST /v0/workspaces/:wsId/git/stage. It stages either the listed
// paths or, when hunkId is present, the single hunk on path. The body must carry
// paths[] or (path + hunkId); neither yields 400. The response echoes the
// workspace id.
func (h *Handlers) Stage(
	c *gin.Context,
) {
	var body stageRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if !body.valid() {
		libs.WriteErr(c, http.StatusBadRequest, "paths or (path and hunkId) is required")
		return
	}
	if err := h.stage(c, body); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}

func (h *Handlers) stage(
	c *gin.Context,
	body stageRequest,
) error {
	wsID := c.Param("wsId")
	if body.isHunk() {
		return h.git.StageHunk(c.Request.Context(), wsID, body.Path, body.HunkID, h.now())
	}
	for _, p := range body.Paths {
		if err := h.git.StageFile(c.Request.Context(), wsID, p, h.now()); err != nil {
			return err
		}
	}
	return nil
}

// Unstage handles POST /v0/workspaces/:wsId/git/unstage. It unstages either the
// listed paths or, when hunkId is present, the single hunk on path. The body
// must carry paths[] or (path + hunkId); neither yields 400. The response echoes
// the workspace id.
func (h *Handlers) Unstage(
	c *gin.Context,
) {
	var body stageRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if !body.valid() {
		libs.WriteErr(c, http.StatusBadRequest, "paths or (path and hunkId) is required")
		return
	}
	if err := h.unstage(c, body); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}

func (h *Handlers) unstage(
	c *gin.Context,
	body stageRequest,
) error {
	wsID := c.Param("wsId")
	if body.isHunk() {
		return h.git.UnstageHunk(c.Request.Context(), wsID, body.Path, body.HunkID, h.now())
	}
	for _, p := range body.Paths {
		if err := h.git.UnstageFile(c.Request.Context(), wsID, p, h.now()); err != nil {
			return err
		}
	}
	return nil
}

// Discard handles POST /v0/workspaces/:wsId/git/discard, reverting the
// working-tree changes of the listed paths. The body requires paths[]; an empty
// list yields 400. The response echoes the workspace id.
func (h *Handlers) Discard(
	c *gin.Context,
) {
	var body discardRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Paths) == 0 {
		libs.WriteErr(c, http.StatusBadRequest, "paths is required")
		return
	}
	if err := h.discard(c, body); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}

func (h *Handlers) discard(
	c *gin.Context,
	body discardRequest,
) error {
	wsID := c.Param("wsId")
	for _, p := range body.Paths {
		if err := h.git.Discard(c.Request.Context(), wsID, p, h.now()); err != nil {
			return err
		}
	}
	return nil
}
