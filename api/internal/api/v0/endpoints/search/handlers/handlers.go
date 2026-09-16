// Package handlers holds the gin handlers backing the search endpoint: global
// workspace search and in-place replace.
package handlers

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
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

// Handlers serves the search and replace routes from the search engine,
// mounted on /v0/chats/:chatId/search... (routes.go).
type Handlers struct {
	searchEng SearchEngine
}

// New builds the search Handlers from the search engine.
func New(
	searchEng SearchEngine,
) *Handlers {
	return &Handlers{
		searchEng: searchEng,
	}
}

// resolveWorkspace answers which workspace this request acts on: the chat
// group's resolveChatWorktree middleware has already resolved the chat's
// worktree and stashed it on the context, so the answer is read back from
// reqscope — never resolved a second time per request, and never taken from a
// URL, because no chat-scoped URL carries a workspace id to take it from
// (spec law 1).
func (h *Handlers) resolveWorkspace(
	ctx *gin.Context,
) (domain.Workspace, error) {
	if ws, ok := reqscope.Workspace(ctx); ok {
		return ws, nil
	}
	return domain.Workspace{}, apperr.ErrNotFound
}
