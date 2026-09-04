package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

// Search handles POST /v0/chats/:chatId/search.
func (h *Handlers) Search(
	ctx *gin.Context,
) {
	if h.searchEng == nil {
		libs.WriteErr(ctx, http.StatusServiceUnavailable, "search engine not available")
		return
	}

	ws, err := h.resolveWorkspace(ctx)
	if err != nil {
		libs.WriteErr(ctx, http.StatusNotFound, "workspace not found")
		return
	}

	var body struct {
		Query         string   `json:"query"`
		CaseSensitive bool     `json:"caseSensitive"`
		WholeWord     bool     `json:"wholeWord"`
		Regex         bool     `json:"regex"`
		Include       []string `json:"include"`
		Exclude       []string `json:"exclude"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if body.Query == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "query is required")
		return
	}

	resp, err := h.searchEng.Search(ctx.Request.Context(), ws.WorktreePath, enginesearch.SearchRequest{
		Query:         body.Query,
		CaseSensitive: body.CaseSensitive,
		WholeWord:     body.WholeWord,
		Regex:         body.Regex,
		Include:       body.Include,
		Exclude:       body.Exclude,
	})
	if err != nil {
		handleSearchError(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, resp)
}

// Replace handles POST /v0/chats/:chatId/search/replace.
func (h *Handlers) Replace(
	ctx *gin.Context,
) {
	if h.searchEng == nil {
		libs.WriteErr(ctx, http.StatusServiceUnavailable, "search engine not available")
		return
	}

	ws, err := h.resolveWorkspace(ctx)
	if err != nil {
		libs.WriteErr(ctx, http.StatusNotFound, "workspace not found")
		return
	}

	var body struct {
		Query         string `json:"query"`
		Replacement   string `json:"replacement"`
		Scope         string `json:"scope"`
		CaseSensitive bool   `json:"caseSensitive"`
		WholeWord     bool   `json:"wholeWord"`
		Regex         bool   `json:"regex"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	err = h.searchEng.Replace(ctx.Request.Context(), ws.WorktreePath, enginesearch.ReplaceRequest{
		Query:         body.Query,
		Replacement:   body.Replacement,
		Scope:         body.Scope,
		CaseSensitive: body.CaseSensitive,
		WholeWord:     body.WholeWord,
		Regex:         body.Regex,
	}, ws.Status == domain.WorkspaceStatusLocked)
	if err != nil {
		handleReplaceError(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}

// handleSearchError maps Search errors to appropriate HTTP responses.
func handleSearchError(
	ctx *gin.Context,
	err error,
) {
	if errors.Is(err, enginesearch.ErrBadPattern) {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	libs.WriteErr(ctx, http.StatusInternalServerError, "search failed")
}

// handleReplaceError maps Replace errors to appropriate HTTP responses.
func handleReplaceError(
	ctx *gin.Context,
	err error,
) {
	switch {
	case errors.Is(err, enginesearch.ErrLocked):
		libs.WriteErr(ctx, http.StatusForbidden, "workspace is locked")
	case errors.Is(err, enginesearch.ErrPathOutsideWorkspace):
		libs.WriteErr(ctx, http.StatusBadRequest, "replace path is outside the workspace")
	case errors.Is(err, enginesearch.ErrBadPattern):
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
	default:
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
	}
}
