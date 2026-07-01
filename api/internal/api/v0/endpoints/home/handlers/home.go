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
	// Home workspaces carry no git-merge-eligibility context.
	libs.WriteQueryWithStatus(c, http.StatusOK, dto.WorkspaceDTOFrom(ws, wsrepo.MergeEligibility{}))
}
