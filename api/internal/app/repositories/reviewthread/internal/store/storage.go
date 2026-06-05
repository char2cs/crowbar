package store

import (
	"context"
	"encoding/json"
	"fmt"

	gormdb "gorm.io/gorm"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type reviewThreadRow struct {
	ID   string `gorm:"primaryKey;column:id"`
	Data []byte `gorm:"column:data"`
}

func (reviewThreadRow) TableName() string {
	return "read_review_threads"
}

type storage interface {
	Save(
		ctx context.Context,
		th domain.ReviewThread,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.ReviewThread, error)
	FindAll(
		ctx context.Context,
	) ([]domain.ReviewThread, error)
}

type storageStore struct {
	inner interface {
		Save(ctx context.Context, row reviewThreadRow) error
		Delete(ctx context.Context, key string) error
		FindByKey(ctx context.Context, key string) (*reviewThreadRow, error)
		FindAll(ctx context.Context) ([]reviewThreadRow, error)
	}
}

func newStorageStore(
	db *gormdb.DB,
) (storage, error) {
	inner, err := storesqlite.NewFromDB[reviewThreadRow, string](db)
	if err != nil {
		return nil, fmt.Errorf("reviewthread storage: %w", err)
	}
	return &storageStore{inner: inner}, nil
}

func (s *storageStore) Save(
	ctx context.Context,
	th domain.ReviewThread,
) error {
	data, err := json.Marshal(th)
	if err != nil {
		return fmt.Errorf("reviewthread storage: marshal: %w", err)
	}
	return s.inner.Save(ctx, reviewThreadRow{ID: th.ID, Data: data})
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
) (*domain.ReviewThread, error) {
	row, err := s.inner.FindByKey(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reviewthread storage: find: %w", err)
	}
	if row == nil {
		return nil, nil
	}
	return unmarshalReviewThread(row.Data)
}

func (s *storageStore) FindAll(
	ctx context.Context,
) ([]domain.ReviewThread, error) {
	rows, err := s.inner.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("reviewthread storage: find all: %w", err)
	}
	result := make([]domain.ReviewThread, 0, len(rows))
	for _, row := range rows {
		th, err := unmarshalReviewThread(row.Data)
		if err != nil {
			return nil, err
		}
		result = append(result, *th)
	}
	return result, nil
}

func unmarshalReviewThread(
	data []byte,
) (*domain.ReviewThread, error) {
	var th domain.ReviewThread
	if err := json.Unmarshal(data, &th); err != nil {
		return nil, fmt.Errorf("reviewthread storage: unmarshal: %w", err)
	}
	return &th, nil
}
