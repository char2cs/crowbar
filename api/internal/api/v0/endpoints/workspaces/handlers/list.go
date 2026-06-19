package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// List handles
// GET /v0/projects/:projectId/repos/:repoId/workspaces, returning the
// repo-scoped WorkspaceDTO[] list. The scope is taken from the :projectId and
// :repoId path params and applied conjunctively over the workspace set; the
// repo-filtered slice IS the sibling set used to compute each row's merge
// eligibility (CanMergeLocally/ParentBranch).
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
		c.Param("projectId"),
		c.Param("repoId"),
	)
	libs.WriteQueryOK(c, dto.WorkspaceDTOList(filtered, h.eligibilityIn(filtered)))
}

// Detail handles
// GET /v0/projects/:projectId/repos/:repoId/workspaces/:wsId, returning a
// single WorkspaceDTO with its merge eligibility computed against the same-repo
// siblings.
func (h *Handlers) Detail(
	c *gin.Context,
) {
	ws, err := h.reader.Get(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	siblings, err := h.siblingsOf(c, ws)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	elig := h.reader.MergeEligibilityFor(ws, siblings)
	libs.WriteQueryOK(c, dto.WorkspaceDTOFrom(ws, elig))
}

// eligibilityIn returns a per-row eligibility resolver bound to the given
// sibling set, suitable for WorkspaceDTOList.
func (h *Handlers) eligibilityIn(
	siblings []domain.Workspace,
) func(domain.Workspace) workspace.MergeEligibility {
	return func(ws domain.Workspace) workspace.MergeEligibility {
		return h.reader.MergeEligibilityFor(ws, siblings)
	}
}

// siblingsOf loads the same-repo workspace set for ws so Detail can compute its
// merge eligibility against the rows the list view would scope to.
func (h *Handlers) siblingsOf(
	c *gin.Context,
	ws domain.Workspace,
) ([]domain.Workspace, error) {
	rows, err := h.reader.List(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return filterWorkspaces(rows, ws.ProjectID, ws.RepoID), nil
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
