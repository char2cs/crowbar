package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

const defaultLogLimit = 50

// Log handles GET /v0/workspaces/:wsId/git/log, returning a paginated CommitDTO
// list from HEAD. The limit (default 50) and skip (default 0) query parameters
// page the result; either must be a non-negative integer or the request is
// rejected with 400.
func (h *Handlers) Log(
	c *gin.Context,
) {
	limit, ok := parseNonNegative(c.Query("limit"), defaultLogLimit)
	if !ok {
		libs.WriteErr(c, http.StatusBadRequest, "invalid limit")
		return
	}
	skip, ok := parseNonNegative(c.Query("skip"), 0)
	if !ok {
		libs.WriteErr(c, http.StatusBadRequest, "invalid skip")
		return
	}
	commits, err := h.git.Log(c.Request.Context(), c.Param("wsId"), limit, skip)
	if err != nil {
		code, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, code, msg)
		return
	}
	libs.WriteQueryOK(c, dto.CommitDTOList(commits))
}

func parseNonNegative(
	raw string,
	fallback int,
) (int, bool) {
	if raw == "" {
		return fallback, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
