package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ProtectedBranches handles GET /v0/repos/:id/protected-branches, resolving the
// repo root from the repository whose id is :id and returning its protected
// branch names. The id is a repo id (clients obtain it from GET /v0/repos); an
// unknown id yields a 404. A disabled capability still yields a 200 envelope
// carrying the default protected branches. The data field is an array, empty
// for a repo with no protected branches.
func (h *Handlers) ProtectedBranches(
	c *gin.Context,
) {
	if !h.requireProvider(c) {
		return
	}
	repo, ok := h.repo(c)
	if !ok {
		return
	}
	branches, err := h.provider.ProtectedBranches(
		c.Request.Context(),
		repo.Path,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.ProtectedBranchList(branches))
}

func (h *Handlers) repo(
	c *gin.Context,
) (domain.Repository, bool) {
	row, err := h.repos.FindByKey(
		c.Request.Context(),
		c.Param("id"),
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return domain.Repository{}, false
	}
	if row == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return domain.Repository{}, false
	}
	return *row, true
}
