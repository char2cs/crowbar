package worktree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type WorkspaceReader interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}
