package store

import (
	"context"
	"encoding/json"
	"fmt"

	gormdb "gorm.io/gorm"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type chatRow struct {
	ID   string `gorm:"primaryKey;column:id"`
	Data []byte `gorm:"column:data"`
}

func (chatRow) TableName() string {
	return "read_chats"
}

type storage interface {
	Save(
		ctx context.Context,
		c domain.Chat,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Chat, error)
	FindAll(
		ctx context.Context,
	) ([]domain.Chat, error)
}

type storageStore struct {
	inner interface {
		Save(ctx context.Context, row chatRow) error
		Delete(ctx context.Context, key string) error
		FindByKey(ctx context.Context, key string) (*chatRow, error)
		FindAll(ctx context.Context) ([]chatRow, error)
	}
}

func newStorageStore(
	db *gormdb.DB,
) (storage, error) {
	inner, err := storesqlite.NewFromDB[chatRow, string](db)
	if err != nil {
		return nil, fmt.Errorf("chat storage: %w", err)
	}
	return &storageStore{inner: inner}, nil
}

func (s *storageStore) Save(
	ctx context.Context,
	c domain.Chat,
) error {
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("chat storage: marshal: %w", err)
	}
	return s.inner.Save(ctx, chatRow{ID: c.ID, Data: data})
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
) (*domain.Chat, error) {
	row, err := s.inner.FindByKey(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("chat storage: find: %w", err)
	}
	if row == nil {
		return nil, nil
	}
	return unmarshalChat(row.Data)
}

func (s *storageStore) FindAll(
	ctx context.Context,
) ([]domain.Chat, error) {
	rows, err := s.inner.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("chat storage: find all: %w", err)
	}
	result := make([]domain.Chat, 0, len(rows))
	for _, row := range rows {
		c, err := unmarshalChat(row.Data)
		if err != nil {
			return nil, err
		}
		result = append(result, *c)
	}
	return result, nil
}

func unmarshalChat(
	data []byte,
) (*domain.Chat, error) {
	var c domain.Chat
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("chat storage: unmarshal: %w", err)
	}
	return &c, nil
}
