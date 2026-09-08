package store

import (
	"context"
	"encoding/json"
	"fmt"

	gormdb "gorm.io/gorm"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// nodeRow is the durable read-model row for one Node, persisted to
// nodes_read. ParentID is carried as its own indexed column even though
// ListByParent currently filters in Go over the full read model (siblings-per-
// parent are few, mirroring agentChatRow/WorkspaceID) — the index positions the
// table for a WHERE parent_id = ? query if that assumption ever stops holding.
type nodeRow struct {
	ID       string `gorm:"primaryKey;column:id"`
	ParentID string `gorm:"column:parent_id;index"`
	Data     []byte `gorm:"column:data"`
}

func (nodeRow) TableName() string {
	return "nodes_read"
}

type storage interface {
	Save(
		ctx context.Context,
		node domain.Node,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Node, error)
	FindAll(
		ctx context.Context,
	) ([]domain.Node, error)
}

type storageStore struct {
	inner interface {
		Save(ctx context.Context, row nodeRow) error
		Delete(ctx context.Context, key string) error
		FindByKey(ctx context.Context, key string) (*nodeRow, error)
		FindAll(ctx context.Context) ([]nodeRow, error)
	}
}

func newStorageStore(
	db *gormdb.DB,
) (storage, error) {
	inner, err := storesqlite.NewFromDB[nodeRow, string](db)
	if err != nil {
		return nil, fmt.Errorf("node storage: %w", err)
	}
	return &storageStore{inner: inner}, nil
}

func (s *storageStore) Save(
	ctx context.Context,
	node domain.Node,
) error {
	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("node storage: marshal: %w", err)
	}
	return s.inner.Save(ctx, nodeRow{ID: node.ID, ParentID: node.ParentID, Data: data})
}

func (s *storageStore) Delete(
	ctx context.Context,
	id string,
) error {
	return s.inner.Delete(ctx, id)
}

func (s *storageStore) FindByKey(
	ctx context.Context,
	id string,
) (*domain.Node, error) {
	row, err := s.inner.FindByKey(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("node storage: find: %w", err)
	}
	if row == nil {
		return nil, nil
	}
	return unmarshalNode(row.Data)
}

func (s *storageStore) FindAll(
	ctx context.Context,
) ([]domain.Node, error) {
	rows, err := s.inner.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("node storage: find all: %w", err)
	}
	result := make([]domain.Node, 0, len(rows))
	for _, row := range rows {
		node, err := unmarshalNode(row.Data)
		if err != nil {
			return nil, err
		}
		result = append(result, *node)
	}
	return result, nil
}

func unmarshalNode(
	data []byte,
) (*domain.Node, error) {
	var node domain.Node
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("node storage: unmarshal: %w", err)
	}
	return &node, nil
}
