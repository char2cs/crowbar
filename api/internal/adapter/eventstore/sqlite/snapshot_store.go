package sqlite

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
)

type snapshotEntry struct {
	AggregateID string `gorm:"primaryKey;column:aggregate_id"`
	Version     int64  `gorm:"not null;column:version"`
	Data        []byte `gorm:"not null;column:data"`
}

func (snapshotEntry) TableName() string {
	return "snapshots"
}

type snapshotStore struct {
	db *gorm.DB
}

func NewSnapshotStore(
	path string,
) (models.SnapshotStore, error) {
	if err := storesqlite.OwnerOnly(path); err != nil {
		return nil, fmt.Errorf("snapshotstore: %w", err)
	}
	db, err := gorm.Open(sqlite.Open(storesqlite.DSN(path)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("snapshotstore: open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("snapshotstore: db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		return nil, fmt.Errorf("snapshotstore: journal_mode: %w", err)
	}
	if err := db.Exec("PRAGMA busy_timeout=5000").Error; err != nil {
		return nil, fmt.Errorf("snapshotstore: busy_timeout: %w", err)
	}

	if err := db.AutoMigrate(&snapshotEntry{}); err != nil {
		return nil, fmt.Errorf("snapshotstore: migrate: %w", err)
	}

	return &snapshotStore{db: db}, nil
}

func (s *snapshotStore) Put(
	ctx context.Context,
	aggregateID string,
	version int64,
	data []byte,
) error {
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "aggregate_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"version", "data"}),
		Where: clause.Where{Exprs: []clause.Expression{
			gorm.Expr("excluded.version > snapshots.version"),
		}},
	}).Create(&snapshotEntry{
		AggregateID: aggregateID,
		Version:     version,
		Data:        data,
	})
	if result.Error != nil {
		return fmt.Errorf("snapshotstore: put: %w", result.Error)
	}
	return nil
}

func (s *snapshotStore) Get(
	ctx context.Context,
	aggregateID string,
) ([]byte, bool, error) {
	var entry snapshotEntry
	err := s.db.WithContext(ctx).
		Where("aggregate_id = ?", aggregateID).
		First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("snapshotstore: get: %w", err)
	}
	return entry.Data, true, nil
}

func (s *snapshotStore) Delete(
	ctx context.Context,
	aggregateID string,
) error {
	result := s.db.WithContext(ctx).
		Where("aggregate_id = ?", aggregateID).
		Delete(&snapshotEntry{})
	if result.Error != nil {
		return fmt.Errorf("snapshotstore: delete: %w", result.Error)
	}
	return nil
}

func (s *snapshotStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("snapshotstore: close: %w", err)
	}
	return sqlDB.Close()
}
