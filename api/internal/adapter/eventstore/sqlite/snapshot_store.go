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

// NewSnapshotStore returns a GORM-backed asynx SnapshotStore at path.
//
// Location decision: snapshots get their OWN db file (one per aggregate type,
// e.g. state/events/workspace_snapshots.db), never a table inside the event
// log's db. The event log is opened single-connection write-pinned (see
// NewEventStore), so a snapshots table sharing that db would share that one
// connection — and the workspace-scoped read hot path (workspace.Get ->
// Reader.Load) issues SnapshotStore.Get FIRST, before touching events. Sharing
// the connection would serialize that O(1) Get behind any in-flight event
// Append. asynx already decouples the two writes — the writer appends the event
// (the durable save point) and then Puts the snapshot as a best-effort,
// non-rolled-back follow-up — so there is no cross-store transaction to
// preserve by co-locating them. A separate file gives the read path its own
// connection (no head-of-line blocking) at the cost of one extra file+handle
// per type, and it is architecturally honest: a snapshot is a derived cache
// that can be dropped and rebuilt by replaying events, so it lives apart from
// the source-of-truth log. Pins to a single connection (serialized upserts),
// enables WAL with a 5-second busy timeout, and checkpoints the WAL on Close —
// mirroring NewEventStore.
func NewSnapshotStore(
	path string,
) (models.SnapshotStore, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
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

// Put upserts the snapshot for aggregateID: the primary key is aggregate_id
// alone, so exactly one row exists per aggregate at all times. The upsert is
// guarded by a monotonicity check — ON CONFLICT(aggregate_id) DO UPDATE ...
// WHERE excluded.version > snapshots.version — so a stale Put (e.g. a slow cold
// replay racing a newer command write) can never overwrite a newer snapshot
// with an older one. The guard is optional per the SnapshotStore contract
// (asynx tolerates last-write-wins, only paying extra replay), but it is cheap
// and turns an out-of-order write into a no-op rather than a correctness cost.
// A conflicting Put that fails the guard affects zero rows and returns no
// error, which matches the contract.
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

// Get returns the stored snapshot for aggregateID. found is false when no
// snapshot exists yet — the normal state of a never-snapshotted aggregate,
// which the contract requires be reported as (nil, false, nil), not an error.
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

// Delete removes the snapshot for aggregateID. Idempotent — deleting a
// non-existent aggregateID affects zero rows and is not an error.
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

// Close checkpoints the WAL and releases the file handle.
func (s *snapshotStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("snapshotstore: close: %w", err)
	}
	return sqlDB.Close()
}
