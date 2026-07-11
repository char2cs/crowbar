package handlers

import (
	"context"

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
	filtered, err := h.reader.ListInRepo(c.Request.Context(), c.Param("projectId"), c.Param("repoId"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	h.applyWorking(filtered)
	libs.WriteQueryOK(c, dto.WorkspaceDTOList(filtered, h.eligibilityIn(c.Request.Context(), filtered)))
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
	elig := h.reader.MergeEligibilityFor(c.Request.Context(), ws, siblings)
	ws.Working = h.working.WorkingFor(ws.ID)
	libs.WriteQueryOK(c, dto.WorkspaceDTOFrom(ws, elig))
}

// applyWorking stamps the derived working overlay onto the rows so REST reads
// agree with the live broadcast frames — WorkingFor combines BOTH overlays
// (inflight background mutation AND agent chat mid-turn) so a list fetched during
// either kind of activity matches the WS stream and snapshot readers.
func (h *Handlers) applyWorking(
	rows []domain.Workspace,
) {
	for i := range rows {
		rows[i].Working = h.working.WorkingFor(rows[i].ID)
	}
}

// eligibilityIn returns a per-row eligibility resolver bound to the given
// sibling set and request context, suitable for WorkspaceDTOList.
func (h *Handlers) eligibilityIn(
	ctx context.Context,
	siblings []domain.Workspace,
) func(domain.Workspace) workspace.MergeEligibility {
	return func(ws domain.Workspace) workspace.MergeEligibility {
		return h.reader.MergeEligibilityFor(ctx, ws, siblings)
	}
}

// siblingsOf loads the same-repo workspace set for ws so Detail can compute its
// merge eligibility against the rows the list view would scope to.
func (h *Handlers) siblingsOf(
	c *gin.Context,
	ws domain.Workspace,
) ([]domain.Workspace, error) {
	return h.reader.ListInRepo(c.Request.Context(), ws.ProjectID, ws.RepoID)
}
