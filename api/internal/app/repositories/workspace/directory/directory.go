// Package directory provides a queryable, rebuildable projection of workspace
// rows scoped by project and repo. The workspace aggregate itself is
// event-sourced per entity (one event_stream.db + view.db per workspace, so
// deleting a workspace is a clean directory removal); that per-entity storage
// has no way to answer "every workspace in repo R" without opening every
// entity in the whole install. This package holds a denormalized copy of
// every workspace row, keyed for that one query, in the shared global
// view.db. The per-entity stores remain the sole source of truth — this
// table is derived and safe to wipe and Rebuild at any time.
package directory

import (
	"context"
	"encoding/json"
	"fmt"

	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Row is the GORM model backing the workspace_directory table: the indexed
// scope columns a query needs, plus the full workspace JSON-marshaled so a
// read never drops a field the domain type gains later.
type Row struct {
	ID        string `gorm:"primaryKey;column:id"`
	ProjectID string `gorm:"column:project_id;index:idx_workspace_directory_scope,priority:1"`
	RepoID    string `gorm:"column:repo_id;index:idx_workspace_directory_scope,priority:2"`
	ParentID  string `gorm:"column:parent_id;index"`
	Data      []byte `gorm:"column:data"`
}

// TableName pins the table name explicitly (GORM would otherwise pluralize
// Row to "rows", which collides with the SQL keyword).
func (Row) TableName() string {
	return "workspace_directory"
}

// Directory is the queryable, rebuildable workspace projection. The
// per-entity event-sourced store remains authoritative; Directory only serves
// list/tree/snapshot reads.
type Directory interface {
	// Upsert writes ws's current row. Called on every workspace broadcast.
	Upsert(ctx context.Context, ws domain.Workspace) error
	// Delete removes id's row. Called on a workspace's deleted tombstone.
	Delete(ctx context.Context, id string) error
	// ListByRepo returns every workspace matching projectID and repoID. An
	// empty component matches every value at that level (mirroring
	// api/v0's scopeWorkspacesToRepo semantics it replaces), so a
	// project-level or blank scope still returns the wider set.
	ListByRepo(ctx context.Context, projectID string, repoID string) ([]domain.Workspace, error)
	// Rebuild atomically replaces the entire table's contents with all. Used
	// once at Container construction to seed/reconcile the projection from a
	// full per-entity scan, and safe to call again anytime as a recovery
	// action since the table is fully derived.
	Rebuild(ctx context.Context, all []domain.Workspace) error
}

type gormDirectory struct {
	db *gormdb.DB
}

// New opens the workspace_directory table on db (the shared global view.db),
// auto-migrating its schema.
func New(
	db *gormdb.DB,
) (Directory, error) {
	if err := db.AutoMigrate(&Row{}); err != nil {
		return nil, fmt.Errorf("directory: migrate: %w", err)
	}
	return &gormDirectory{db: db}, nil
}

func toRow(
	ws domain.Workspace,
) (Row, error) {
	data, err := json.Marshal(ws)
	if err != nil {
		return Row{}, fmt.Errorf("directory: marshal: %w", err)
	}
	return Row{
		ID:        ws.ID,
		ProjectID: ws.ProjectID,
		RepoID:    ws.RepoID,
		ParentID:  ws.ParentID,
		Data:      data,
	}, nil
}

func fromRow(
	row Row,
) (domain.Workspace, error) {
	var ws domain.Workspace
	if err := json.Unmarshal(row.Data, &ws); err != nil {
		return domain.Workspace{}, fmt.Errorf("directory: unmarshal: %w", err)
	}
	return ws, nil
}

func fromRows(
	rows []Row,
) ([]domain.Workspace, error) {
	out := make([]domain.Workspace, 0, len(rows))
	for _, row := range rows {
		ws, err := fromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, ws)
	}
	return out, nil
}

func (g *gormDirectory) Upsert(
	ctx context.Context,
	ws domain.Workspace,
) error {
	row, err := toRow(ws)
	if err != nil {
		return err
	}
	if err := g.db.WithContext(ctx).Save(&row).Error; err != nil {
		return fmt.Errorf("directory: upsert: %w", err)
	}
	return nil
}

func (g *gormDirectory) Delete(
	ctx context.Context,
	id string,
) error {
	if err := g.db.WithContext(ctx).Where("id = ?", id).Delete(&Row{}).Error; err != nil {
		return fmt.Errorf("directory: delete: %w", err)
	}
	return nil
}

func (g *gormDirectory) ListByRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	q := g.db.WithContext(ctx)
	if projectID != "" {
		q = q.Where("project_id = ?", projectID)
	}
	if repoID != "" {
		q = q.Where("repo_id = ?", repoID)
	}
	var rows []Row
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("directory: list by repo: %w", err)
	}
	return fromRows(rows)
}

func (g *gormDirectory) Rebuild(
	ctx context.Context,
	all []domain.Workspace,
) error {
	return g.db.WithContext(ctx).Transaction(func(tx *gormdb.DB) error {
		if err := tx.Where("1 = 1").Delete(&Row{}).Error; err != nil {
			return fmt.Errorf("directory: rebuild: clear: %w", err)
		}
		for _, ws := range all {
			row, err := toRow(ws)
			if err != nil {
				return err
			}
			if err := tx.Create(&row).Error; err != nil {
				return fmt.Errorf("directory: rebuild: insert %s: %w", ws.ID, err)
			}
		}
		return nil
	})
}
