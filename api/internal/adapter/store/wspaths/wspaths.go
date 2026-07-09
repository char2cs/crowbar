// Package wspaths is the workspace id↔path index living in the adapter's
// view.db. It maps a workspace ID to its last-known worktree path so a
// workspace still resolves after its derived, human-readable path has drifted
// (e.g. a branch rename that moved the worktree on disk). The row is written on
// workspace Create, updated on rename, and deleted when the delete reactor
// purges the aggregate. It is deliberately distinct from the store/workspace.db
// read model (which carries project/repo identity for listing and merge
// eligibility): this index exists only for rename resilience.
package wspaths

import (
	"context"
	"errors"
	"fmt"

	gormdb "gorm.io/gorm"
)

// ErrNotFound is returned by Get when no path row exists for a workspace ID.
// It is a package-local sentinel; the app-boundary translation to
// apperr.ErrNotFound is the caller's responsibility.
var ErrNotFound = errors.New("wspaths: not found")

// WorkspacePaths is the workspace id↔path index. It is a plain-CRUD store over
// the adapter's view.db, exposing only the narrow Put/Get/Delete surface the
// rename-resilience map needs.
type WorkspacePaths interface {
	// Put records (upserting) the worktree path for a workspace ID.
	Put(
		ctx context.Context,
		workspaceID string,
		worktreePath string,
	) error
	// Get returns the last-known worktree path for a workspace ID, or
	// ErrNotFound (wrapped) when no row exists for it.
	Get(
		ctx context.Context,
		workspaceID string,
	) (string, error)
	// Delete removes the path row for a workspace ID. It is idempotent: a
	// missing row is not an error.
	Delete(
		ctx context.Context,
		workspaceID string,
	) error
}

type workspacePath struct {
	WorkspaceID  string `gorm:"primaryKey;column:workspace_id"`
	WorktreePath string `gorm:"column:worktree_path;not null"`
}

func (workspacePath) TableName() string {
	return "workspace_paths"
}

type gormStore struct {
	db *gormdb.DB
}

// NewWorkspacePaths builds the workspace id↔path index over the adapter's
// view.db, auto-migrating the workspace_paths table.
func NewWorkspacePaths(
	db *gormdb.DB,
) (WorkspacePaths, error) {
	if err := db.AutoMigrate(&workspacePath{}); err != nil {
		return nil, fmt.Errorf("wspaths: migrate: %w", err)
	}
	return &gormStore{db: db}, nil
}

func (s *gormStore) Put(
	ctx context.Context,
	workspaceID string,
	worktreePath string,
) error {
	row := workspacePath{WorkspaceID: workspaceID, WorktreePath: worktreePath}
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return fmt.Errorf("wspaths: put: %w", err)
	}
	return nil
}

func (s *gormStore) Get(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	var row workspacePath
	err := s.db.WithContext(ctx).First(&row, "workspace_id = ?", workspaceID).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return "", fmt.Errorf("wspaths: get %q: %w", workspaceID, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("wspaths: get: %w", err)
	}
	return row.WorktreePath, nil
}

func (s *gormStore) Delete(
	ctx context.Context,
	workspaceID string,
) error {
	if err := s.db.WithContext(ctx).Delete(&workspacePath{}, "workspace_id = ?", workspaceID).Error; err != nil {
		return fmt.Errorf("wspaths: delete: %w", err)
	}
	return nil
}
