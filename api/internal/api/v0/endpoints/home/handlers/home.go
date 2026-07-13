package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	wsrepo "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
)

// Get handles GET /v0/projects/:projectId/home.
// It returns the home workspace DTO for the project.
func (h *Handlers) Get(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	// Stamp the derived working overlay from the same seam the workspaces
	// handlers use, so this REST read agrees with the home workspace's live
	// broadcast frames (which the container enriches via the same WorkingFor):
	// a project-home read taken mid-agent-turn reports working=true and the
	// home workspace's icon keeps its spinner across a refetch.
	ws.Working = h.working.WorkingFor(ws.ID)
	// Home workspaces carry no git-merge-eligibility context.
	libs.WriteQueryWithStatus(c, http.StatusOK, dto.WorkspaceDTOFrom(ws, wsrepo.MergeEligibility{}))
}
