package handlers

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

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

type Handlers struct {
	searchEng SearchEngine
}

func New(
	searchEng SearchEngine,
) *Handlers {
	return &Handlers{
		searchEng: searchEng,
	}
}

func (h *Handlers) resolveWorkspace(
	ctx *gin.Context,
) (domain.Workspace, error) {
	if ws, ok := reqscope.Workspace(ctx); ok {
		return ws, nil
	}
	return domain.Workspace{}, problem.ErrNotFound
}
