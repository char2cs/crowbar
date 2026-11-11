package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/char2cs/asynx/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
)

type eventEntry struct {
	AggregateID string `gorm:"primaryKey;column:aggregate_id"`
	Version     int64  `gorm:"primaryKey;column:version"`
	Data        []byte `gorm:"not null"`
}

func (eventEntry) TableName() string {
	return "events"
}

// The receipts and deliveries tables are new in v1; events keeps its schema.
var v1Tables = []string{
	`CREATE TABLE IF NOT EXISTS receipts (
		command_id TEXT PRIMARY KEY,
		accepted INTEGER NOT NULL,
		aggregate_id TEXT NOT NULL,
		event_id TEXT NOT NULL,
		version INTEGER NOT NULL,
		rejection TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS deliveries (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		aggregate_id TEXT NOT NULL,
		version INTEGER NOT NULL,
		UNIQUE(aggregate_id, version)
	)`,
}

// eventStoreConns lets reads and acks run beside the one open write
// transaction (WAL admits one writer and many readers).
const eventStoreConns = 4

type eventStore struct {
	db *sql.DB
}

// NewEventStore returns an asynx event store over the SQLite database at path.
// Write transactions begin IMMEDIATE so two writers queue on the busy timeout
// instead of failing a lock upgrade.
func NewEventStore(
	path string,
) (models.Store, error) {
	if err := storesqlite.OwnerOnly(path); err != nil {
		return nil, fmt.Errorf("eventstore: %w", err)
	}
	db, err := gorm.Open(sqlite.Open(eventStoreDSN(path)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("eventstore: open: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("eventstore: db: %w", err)
	}
	sqlDB.SetMaxOpenConns(maxConns(path))

	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		return nil, errors.Join(fmt.Errorf("eventstore: journal_mode: %w", err), sqlDB.Close())
	}
	if err := db.AutoMigrate(&eventEntry{}); err != nil {
		return nil, errors.Join(fmt.Errorf("eventstore: migrate: %w", err), sqlDB.Close())
	}
	for _, ddl := range v1Tables {
		if err := db.Exec(ddl).Error; err != nil {
			return nil, errors.Join(fmt.Errorf("eventstore: migrate: %w", err), sqlDB.Close())
		}
	}
	return &eventStore{db: sqlDB}, nil
}

// OpenReadOnly returns an event store over the existing database at path that
// only reads: it creates no table, so a retired log stays as its build wrote it.
func OpenReadOnly(path string) (models.Store, error) {
	db, err := sql.Open(sqlite.DriverName, storesqlite.ReadOnlyDSN(path))
	if err != nil {
		return nil, fmt.Errorf("eventstore: open read-only: %w", err)
	}
	return &eventStore{db: db}, nil
}

func eventStoreDSN(
	path string,
) string {
	return storesqlite.DSN(path) + "&_pragma=busy_timeout(5000)&_txlock=immediate"
}

// An in-memory SQLite database belongs to one connection, so it cannot be pooled.
func maxConns(
	path string,
) int {
	if path == ":memory:" {
		return 1
	}
	return eventStoreConns
}

func (s *eventStore) ReadFrom(
	ctx context.Context,
	streamID string,
	fromVersion int64,
) ([][]byte, error) {
	return s.readBlobs(ctx, "eventstore: read from",
		"SELECT data FROM events WHERE aggregate_id = ? AND version >= ? ORDER BY version",
		streamID, fromVersion)
}

func (s *eventStore) ReadRange(
	ctx context.Context,
	streamID string,
	fromVersion int64,
	count int64,
) ([][]byte, error) {
	return s.readBlobs(ctx, "eventstore: read range",
		"SELECT data FROM events WHERE aggregate_id = ? AND version >= ? ORDER BY version LIMIT ?",
		streamID, fromVersion, count)
}

func (s *eventStore) readBlobs(
	ctx context.Context,
	op string,
	query string,
	args ...any,
) (blobs [][]byte, err error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	blobs = [][]byte{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		blobs = append(blobs, data)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return blobs, nil
}

// AggregateIDs returns the distinct stream ids that hold at least one event.
// It backs the read-model reconcile-on-open for stores holding many aggregates.
func (s *eventStore) AggregateIDs(
	ctx context.Context,
) (ids []string, err error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("eventstore: aggregate ids: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT aggregate_id FROM events")
	if err != nil {
		return nil, fmt.Errorf("eventstore: aggregate ids: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	ids = []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("eventstore: aggregate ids: scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("eventstore: aggregate ids: %w", err)
	}
	return ids, nil
}

func (s *eventStore) GetReceipt(
	ctx context.Context,
	commandID string,
) (models.Receipt, bool, error) {
	if err := ctx.Err(); err != nil {
		return models.Receipt{}, false, fmt.Errorf("eventstore: get receipt: %w", err)
	}
	var r models.Receipt
	err := s.db.QueryRowContext(ctx,
		"SELECT accepted, aggregate_id, event_id, version, rejection FROM receipts WHERE command_id = ?",
		commandID,
	).Scan(&r.Accepted, &r.AggregateID, &r.EventID, &r.Version, &r.Rejection)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Receipt{}, false, nil
	}
	if err != nil {
		return models.Receipt{}, false, fmt.Errorf("eventstore: get receipt: %w", err)
	}
	return r, true, nil
}

func (s *eventStore) PendingDeliveries(
	ctx context.Context,
	limit int,
) (pending []models.Delivery, err error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("eventstore: pending deliveries: %w", err)
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT aggregate_id, version FROM deliveries ORDER BY seq LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("eventstore: pending deliveries: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	pending = []models.Delivery{}
	for rows.Next() {
		var d models.Delivery
		if err := rows.Scan(&d.AggregateID, &d.Version); err != nil {
			return nil, fmt.Errorf("eventstore: pending deliveries: scan: %w", err)
		}
		pending = append(pending, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("eventstore: pending deliveries: %w", err)
	}
	return pending, nil
}

func (s *eventStore) AckDelivery(
	ctx context.Context,
	aggregateID string,
	version int64,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("eventstore: ack delivery: %w", err)
	}
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM deliveries WHERE aggregate_id = ? AND version = ?", aggregateID, version)
	if err != nil {
		return fmt.Errorf("eventstore: ack delivery: %w", err)
	}
	return nil
}

func (s *eventStore) Atomically(
	ctx context.Context,
	fn func(tx models.Tx) error,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("eventstore: atomically: %w", err)
	}
	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("eventstore: begin: %w", err)
	}
	if err := fn(&tx{tx: sqlTx}); err != nil {
		if rbErr := sqlTx.Rollback(); rbErr != nil {
			return errors.Join(err, fmt.Errorf("eventstore: rollback: %w", rbErr))
		}
		return err
	}
	if err := sqlTx.Commit(); err != nil {
		return fmt.Errorf("eventstore: commit: %w", err)
	}
	return nil
}

// Close checkpoints the WAL and releases the file handle.
func (s *eventStore) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("eventstore: close: %w", err)
	}
	return nil
}
