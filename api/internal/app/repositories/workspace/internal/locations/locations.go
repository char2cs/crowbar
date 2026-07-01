// Package locations is the global workspace location index. It maps a workspace
// ID to its (projectID, repoID) so the per-entity event store and view DB can be
// resolved from an ID alone. The index lives in the global view DB, written on
// workspace Create and removed on Delete.
package locations

import (
	"context"
	"errors"
	"fmt"

	gormdb "gorm.io/gorm"
)

// ErrNotFound is returned when no location row exists for an ID.
var ErrNotFound = errors.New("locations: not found")

// Location records where a workspace's per-entity stores live.
type Location struct {
	ID        string
	ProjectID string
	RepoID    string
}

type workspaceLocation struct {
	ID        string `gorm:"primaryKey;column:id"`
	ProjectID string `gorm:"column:project_id"`
	RepoID    string `gorm:"column:repo_id"`
}

func (workspaceLocation) TableName() string {
	return "workspace_locations"
}

// Store is the workspace location index.
type Store interface {
	Save(
		ctx context.Context,
		loc Location,
	) error
	Get(
		ctx context.Context,
		id string,
	) (Location, error)
	List(
		ctx context.Context,
	) ([]Location, error)
	Delete(
		ctx context.Context,
		id string,
	) error
}

type gormStore struct {
	db *gormdb.DB
}

// New builds the location index over the global view DB, auto-migrating the
// workspace_locations table.
func New(
	db *gormdb.DB,
) (Store, error) {
	if err := db.AutoMigrate(&workspaceLocation{}); err != nil {
		return nil, fmt.Errorf("locations: migrate: %w", err)
	}
	return &gormStore{db: db}, nil
}

func (s *gormStore) Save(
	ctx context.Context,
	loc Location,
) error {
	row := workspaceLocation{
		ID:        loc.ID,
		ProjectID: loc.ProjectID,
		RepoID:    loc.RepoID,
	}
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return fmt.Errorf("locations: save: %w", err)
	}
	return nil
}

func (s *gormStore) Get(
	ctx context.Context,
	id string,
) (Location, error) {
	var row workspaceLocation
	err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return Location{}, fmt.Errorf("locations: get %q: %w", id, ErrNotFound)
	}
	if err != nil {
		return Location{}, fmt.Errorf("locations: get: %w", err)
	}
	return Location{ID: row.ID, ProjectID: row.ProjectID, RepoID: row.RepoID}, nil
}

func (s *gormStore) List(
	ctx context.Context,
) ([]Location, error) {
	var rows []workspaceLocation
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("locations: list: %w", err)
	}
	out := make([]Location, 0, len(rows))
	for _, row := range rows {
		out = append(out, Location{ID: row.ID, ProjectID: row.ProjectID, RepoID: row.RepoID})
	}
	return out, nil
}

func (s *gormStore) Delete(
	ctx context.Context,
	id string,
) error {
	if err := s.db.WithContext(ctx).Delete(&workspaceLocation{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("locations: delete: %w", err)
	}
	return nil
}
