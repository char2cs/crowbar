// Package handlers holds the gin handlers backing the search endpoint: global
// workspace search and in-place replace.
package handlers

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
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

// Handlers serves the search and replace routes from the search engine and
// workspace reader, mounted on both /v0/chats/:chatId/search... and the older
// /v0/workspaces/:wsId/search... (routes.go).
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

// resolveWorkspace answers which workspace this request acts on, for either
// of the two groups search is currently mounted on (routes.go).
//
// On /v0/chats/:chatId/search... the chat group's resolveChatWorktree
// middleware has already resolved the chat's worktree and stashed the
// workspace on the context, so the answer is read back from reqscope — never
// resolved a second time per request, and never taken from a URL, because no
// chat-scoped URL carries a workspace id to take it from (spec law 1).
//
// The :wsId branch is the old workspace-scoped mount, unretired until spec §8
// step 6: it still resolves through wsReader, exactly as before this step.
// When that mount goes, so does the branch and the wsReader field with it.
func (h *Handlers) resolveWorkspace(
	ctx *gin.Context,
) (domain.Workspace, error) {
	if ws, ok := reqscope.Workspace(ctx); ok {
		return ws, nil
	}
	return h.wsReader.Get(ctx.Request.Context(), ctx.Param("wsId"))
}
