package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// WorkspacePlacements is a fake project.HomeWorkspaces over a fixed set of
// workspace rows.
type WorkspacePlacements struct {
	Rows []domain.Workspace
}

// NewWorkspacePlacements returns an empty WorkspacePlacements.
func NewWorkspacePlacements() *WorkspacePlacements {
	return &WorkspacePlacements{}
}

// GetHomeForProject mirrors the real workspace repository's own scan: the
// one row in Rows whose ProjectID matches and whose Kind is
// WorkspaceKindHome, or problem.ErrNotFound.
func (s *WorkspacePlacements) GetHomeForProject(
	ctx context.Context,
	projectID string,
) (domain.Workspace, error) {
	for _, w := range s.Rows {
		if w.ProjectID == projectID && w.Kind == domain.WorkspaceKindHome {
			return w, nil
		}
	}
	return domain.Workspace{}, problem.ErrNotFound
}
