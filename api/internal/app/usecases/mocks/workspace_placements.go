package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type WorkspacePlacements struct {
	Rows []domain.Workspace
}

func NewWorkspacePlacements() *WorkspacePlacements {
	return &WorkspacePlacements{}
}

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
