package handlers

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/provider"
)

type ProviderEngine interface {
	PollOnView(
		ctx context.Context,
		wsID string,
		repoPath string,
		branch string,
	) (provider.ProviderState, error)

	ProtectedBranches(
		ctx context.Context,
		repoPath string,
	) ([]string, error)
}

type WorkspaceReader interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
}

type Handlers struct {
	eng      ProviderEngine
	wsReader WorkspaceReader
}

func New(
	eng ProviderEngine,
	wsReader WorkspaceReader,
) *Handlers {
	return &Handlers{
		eng:      eng,
		wsReader: wsReader,
	}
}
