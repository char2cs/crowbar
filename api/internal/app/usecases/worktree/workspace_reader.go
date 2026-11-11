package worktree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// WorkspaceReader can fetch a single workspace by ID.
type WorkspaceReader interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}
