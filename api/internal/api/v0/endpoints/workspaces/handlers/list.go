package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// noEligibility resolves an empty merge-eligibility overlay. The eligibility-
// aware path that reads the repo-scoped sibling set is wired into these handlers
// in W8.3; until then the converters carry the zero overlay.
func noEligibility(
	_ domain.Workspace,
) workspace.MergeEligibility {
	return workspace.MergeEligibility{}
}

// List handles GET /v0/workspaces, returning the flat WorkspaceDTO[] list. An
// optional projectId or repoId query parameter scopes the result to a single
// project or repository; both may be supplied and are applied conjunctively.
func (h *Handlers) List(
	c *gin.Context,
) {
	rows, err := h.reader.List(c.Request.Context())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	filtered := filterWorkspaces(
		rows,
		c.Query("projectId"),
		c.Query("repoId"),
	)
	libs.WriteQueryOK(c, dto.WorkspaceDTOList(filtered, noEligibility))
}

// Detail handles GET /v0/workspaces/:wsId, returning a single WorkspaceDTO.
func (h *Handlers) Detail(
	c *gin.Context,
) {
	ws, err := h.reader.Get(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.WorkspaceDTOFrom(ws, noEligibility(ws)))
}

func filterWorkspaces(
	rows []domain.Workspace,
	projectID string,
	repoID string,
) []domain.Workspace {
	if projectID == "" && repoID == "" {
		return rows
	}
	out := make([]domain.Workspace, 0, len(rows))
	for _, ws := range rows {
		if projectID != "" && ws.ProjectID != projectID {
			continue
		}
		if repoID != "" && ws.RepoID != repoID {
			continue
		}
		out = append(out, ws)
	}
	return out
}
