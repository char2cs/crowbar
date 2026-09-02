package worktree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// WorkspaceReader can fetch a single workspace by ID.
//
// Declared here rather than imported from usecases/workspace (law 3, law 4):
// it mirrors the identical port terminal/handlers.WorkspaceReader already
// declares for the same reason — a consumer names the narrow slice of
// behaviour it needs, and the container satisfies both with the same
// concrete workspace usecase.
type WorkspaceReader interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}
