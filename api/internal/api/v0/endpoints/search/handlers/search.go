package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
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
		failSearch(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, resp)
}

// failSearch answers an uncoded engine failure with a fixed message: it is
// ripgrep's own output, the paths and files it read, not the client's to see.
// The access log keeps the whole chain.
func failSearch(ctx *gin.Context, err error) {
	if problem.CodeOf(err) != problem.Internal {
		libs.Fail(ctx, err)
		return
	}
	libs.NoteError(ctx, err)
	ctx.JSON(http.StatusInternalServerError, libs.Envelope{Success: false, Error: "search failed", Code: problem.Internal})
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
		libs.Fail(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}
