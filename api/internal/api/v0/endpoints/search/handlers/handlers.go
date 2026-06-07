// Package handlers holds the gin handlers backing the search endpoint: the
// global content search and the search-and-replace mutation. Both routes hang
// off /workspaces/:wsId; the workspace id resolves to the worktree path the
// engine walks, and the workspace's Locked flag is forwarded to Replace so a
// provider-protected workspace rejects writes with a 409.
//
// The search engine is guarded for nil before any work runs; an absent engine
// yields a 503. Responses carry the uniform {success,error,data} envelope.
package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

// SearchEngine is the global search/replace surface the handlers consume. It
// mirrors enginesearch.SearchEngine so the live engine satisfies it directly.
type SearchEngine interface {
	Search(
		ctx context.Context,
		repoPath string,
		req enginesearch.SearchRequest,
	) (enginesearch.SearchResponse, error)
	Replace(
		ctx context.Context,
		repoPath string,
		req enginesearch.ReplaceRequest,
		locked bool,
	) error
}

// WorkspaceReader resolves a workspace id to its aggregate, supplying the
// worktree path the engine operates against and the Locked flag forwarded to
// Replace. It mirrors the workspace repository's Get method.
type WorkspaceReader interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

// Handlers serves the /v0 search routes from the search engine and the
// workspace reader. A nil engine surfaces as a 503 on either route.
type Handlers struct {
	eng      SearchEngine
	wsReader WorkspaceReader
}

// New builds the search Handlers from the search engine and the workspace
// reader.
func New(
	eng SearchEngine,
	wsReader WorkspaceReader,
) *Handlers {
	return &Handlers{
		eng:      eng,
		wsReader: wsReader,
	}
}

func (h *Handlers) requireEngine(
	c *gin.Context,
) bool {
	if h.eng == nil {
		libs.WriteErr(
			c,
			http.StatusServiceUnavailable,
			"search engine not available",
		)
		return false
	}
	return true
}

func (h *Handlers) workspace(
	c *gin.Context,
) (domain.Workspace, bool) {
	row, err := h.wsReader.Get(
		c.Request.Context(),
		c.Param("wsId"),
	)
	if err != nil {
		libs.WriteErr(
			c,
			http.StatusNotFound,
			"workspace not found",
		)
		return domain.Workspace{}, false
	}
	return row, true
}
