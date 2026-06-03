package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"

	asynxModels "github.com/char2cs/asynx/models"
	glebsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenDB opens a SQLite GORM database at the given path.
// Sets max open connections to 1 to serialise writes.
func OpenDB(path string) (*gorm.DB, error) {
	db, err := gorm.Open(glebsqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", path, err)
	}

	// db.DB() cannot fail with SQLite after a successful gorm.Open.
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)

	return db, nil
}

// EventStore is a SQLite-backed asynx event store with lifecycle management.
type EventStore interface {
	asynxModels.Store
	// ListAggregateIDs returns all distinct aggregate IDs present in the store.
	ListAggregateIDs(ctx context.Context) ([]string, error)
	io.Closer
}

type eventEntry struct {
	AggregateID string `gorm:"primaryKey;column:aggregate_id"`
	Version     int64  `gorm:"primaryKey;column:version"`
	Data        []byte `gorm:"not null"`
}

func (eventEntry) TableName() string { return "events" }

type eventStore struct {
	db    *gorm.DB
	sqlDB *sql.DB
}

// NewEventStore opens a SQLite event store at path, applying the schema
// migration and serialising writes via SetMaxOpenConns(1).
func NewEventStore(path string) (EventStore, error) {
	db, err := OpenDB(path)
	if err != nil {
		return nil, fmt.Errorf("eventstore: %w", err)
	}

	if err := db.AutoMigrate(&eventEntry{}); err != nil {
		return nil, fmt.Errorf("eventstore: migrate %q: %w", path, err)
	}

	sqlDB, _ := db.DB()
	return &eventStore{db: db, sqlDB: sqlDB}, nil
}

func (s *eventStore) Append(
	ctx context.Context,
	aggregateID string,
	version int64,
	data []byte,
) error {
	result := s.db.WithContext(ctx).Create(&eventEntry{
		AggregateID: aggregateID,
		Version:     version,
		Data:        data,
	})
	if result.Error != nil {
		return fmt.Errorf("%w: version conflict (%s, v%d)", asynxModels.ErrPipelineFailed, aggregateID, version)
	}
	return nil
}

func (s *eventStore) ReadFrom(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
) ([][]byte, error) {
	var entries []eventEntry
	err := s.db.WithContext(ctx).
		Where("aggregate_id = ? AND version >= ?", aggregateID, fromVersion).
		Order("version ASC").
		Find(&entries).Error
	if err != nil {
		return nil, fmt.Errorf("eventstore: read from: %w", err)
	}

	result := make([][]byte, len(entries))
	for i, e := range entries {
		result[i] = e.Data
	}
	return result, nil
}

func (s *eventStore) ReadRange(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
	count int64,
) ([][]byte, error) {
	var entries []eventEntry
	err := s.db.WithContext(ctx).
		Where("aggregate_id = ? AND version >= ?", aggregateID, fromVersion).
		Order("version ASC").
		Limit(int(count)).
		Find(&entries).Error
	if err != nil {
		return nil, fmt.Errorf("eventstore: read range: %w", err)
	}

	result := make([][]byte, len(entries))
	for i, e := range entries {
		result[i] = e.Data
	}
	return result, nil
}

func (s *eventStore) Count(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&eventEntry{}).
		Where("aggregate_id = ? AND version >= ?", aggregateID, fromVersion).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("eventstore: count: %w", err)
	}
	return count, nil
}

func (s *eventStore) Delete(
	ctx context.Context,
	aggregateID string,
) error {
	result := s.db.WithContext(ctx).
		Where("aggregate_id = ?", aggregateID).
		Delete(&eventEntry{})
	if result.Error != nil {
		return fmt.Errorf("eventstore: delete: %w", result.Error)
	}
	return nil
}

// ListAggregateIDs returns all aggregate IDs that have event records.
// Asynx stores rows as "events:<id>"; this method strips the prefix and
// excludes snapshot entries ("snapshots:<id>").
func (s *eventStore) ListAggregateIDs(ctx context.Context) ([]string, error) {
	var rawIDs []string
	err := s.db.WithContext(ctx).
		Model(&eventEntry{}).
		Distinct("aggregate_id").
		Pluck("aggregate_id", &rawIDs).Error
	if err != nil {
		return nil, fmt.Errorf("eventstore: list aggregate ids: %w", err)
	}
	ids := make([]string, 0, len(rawIDs))
	for _, raw := range rawIDs {
		if after, ok := strings.CutPrefix(raw, "events:"); ok {
			ids = append(ids, after)
		}
	}
	return ids, nil
}

// Close closes the underlying database connection, checkpointing the WAL
// and releasing file handles.
func (s *eventStore) Close() error {
	return s.sqlDB.Close()
}
