package v0

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

// registerSearchHandlers mounts the search and replace REST routes on rg.
func registerSearchHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.POST("/workspaces/:wsId/search", c.handleSearch)
	rg.POST("/workspaces/:wsId/search/replace", c.handleReplace)
}

// handleSearch POST /v0/workspaces/:wsId/search
func (c *Container) handleSearch(
	ctx *gin.Context,
) {
	eng := c.requireSearchEngine(ctx)
	if eng == nil {
		return
	}

	wsID := ctx.Param("wsId")
	ws, err := c.app.Repositories.Workspace.Get(ctx.Request.Context(), wsID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
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
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if body.Query == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	resp, err := eng.Search(ctx.Request.Context(), ws.WorktreePath, enginesearch.SearchRequest{
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

	ctx.JSON(http.StatusOK, resp)
}

// handleReplace POST /v0/workspaces/:wsId/search/replace
func (c *Container) handleReplace(
	ctx *gin.Context,
) {
	eng := c.requireSearchEngine(ctx)
	if eng == nil {
		return
	}

	wsID := ctx.Param("wsId")
	ws, err := c.app.Repositories.Workspace.Get(ctx.Request.Context(), wsID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
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
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = eng.Replace(ctx.Request.Context(), ws.WorktreePath, enginesearch.ReplaceRequest{
		Query:         body.Query,
		Replacement:   body.Replacement,
		Scope:         body.Scope,
		CaseSensitive: body.CaseSensitive,
		WholeWord:     body.WholeWord,
		Regex:         body.Regex,
	}, ws.Locked)
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
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": "search failed"})
}

// handleReplaceError maps Replace errors to appropriate HTTP responses.
func handleReplaceError(
	ctx *gin.Context,
	err error,
) {
	switch {
	case errors.Is(err, enginesearch.ErrLocked):
		ctx.JSON(http.StatusForbidden, gin.H{"error": "workspace is locked"})
	case errors.Is(err, enginesearch.ErrPathOutsideWorkspace):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "replace path is outside the workspace"})
	case errors.Is(err, enginesearch.ErrBadPattern):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// requireSearchEngine returns the engine or writes a 503 and returns nil.
func (c *Container) requireSearchEngine(
	ctx *gin.Context,
) enginesearch.SearchEngine {
	if c.eng == nil || c.eng.Search == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "search engine not available"})
		return nil
	}
	return c.eng.Search
}
