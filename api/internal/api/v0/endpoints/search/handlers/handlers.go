// Package handlers holds the gin handlers backing the search endpoint: global
// workspace search and in-place replace.
package handlers

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

// SearchEngine is the global search/replace surface the handlers need.
type SearchEngine interface {
	// Search searches repoPath for the query specified in req.
	Search(
		ctx context.Context,
		repoPath string,
		req enginesearch.SearchRequest,
	) (enginesearch.SearchResponse, error)

	// Replace rewrites matching occurrences on disk.
	Replace(
		ctx context.Context,
		repoPath string,
		req enginesearch.ReplaceRequest,
		locked bool,
	) error
}

// WorkspaceReader is the workspace read surface the handlers need.
type WorkspaceReader interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

// Handlers serves the /v0/workspaces/:wsId/search routes from the search
// engine and workspace reader.
type Handlers struct {
	searchEng SearchEngine
	wsReader  WorkspaceReader
}

// New builds the search Handlers from the search engine and workspace reader.
func New(
	searchEng SearchEngine,
	wsReader WorkspaceReader,
) *Handlers {
	return &Handlers{
		searchEng: searchEng,
		wsReader:  wsReader,
	}
}
