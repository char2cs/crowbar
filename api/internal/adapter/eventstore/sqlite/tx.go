package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/char2cs/asynx/models"
)

// sqliteConstraint is SQLITE_CONSTRAINT; the extended codes share its low byte.
const sqliteConstraint = 19

type tx struct {
	tx *sql.Tx
}

func (t *tx) Native() any { return t.tx }

func (t *tx) Append(
	ctx context.Context,
	streamID string,
	version int64,
	data []byte,
) error {
	if err := t.exec(ctx, "INSERT INTO events (aggregate_id, version, data) VALUES (?, ?, ?)",
		streamID, version, data); err != nil {
		if isConstraint(err) {
			return fmt.Errorf("%w: version conflict (%s, v%d)", models.ErrPipelineFailed, streamID, version)
		}
		return fmt.Errorf("eventstore: append: %w", err)
	}
	return nil
}

func (t *tx) DeleteStream(
	ctx context.Context,
	streamID string,
) error {
	if err := t.exec(ctx, "DELETE FROM events WHERE aggregate_id = ?", streamID); err != nil {
		return fmt.Errorf("eventstore: delete stream: %w", err)
	}
	return nil
}

func (t *tx) DropDeliveries(
	ctx context.Context,
	aggregateID string,
) error {
	if err := t.exec(ctx, "DELETE FROM deliveries WHERE aggregate_id = ?", aggregateID); err != nil {
		return fmt.Errorf("eventstore: drop deliveries: %w", err)
	}
	return nil
}

func (t *tx) PutReceipt(
	ctx context.Context,
	commandID string,
	r models.Receipt,
) error {
	err := t.exec(ctx,
		"INSERT INTO receipts (command_id, accepted, aggregate_id, event_id, version, rejection) VALUES (?, ?, ?, ?, ?, ?)",
		commandID, r.Accepted, r.AggregateID, r.EventID, r.Version, r.Rejection)
	if err == nil {
		return nil
	}
	if isConstraint(err) {
		return fmt.Errorf("%w: %s", models.ErrDuplicateReceipt, commandID)
	}
	return fmt.Errorf("eventstore: put receipt: %w", err)
}

func (t *tx) EnqueueDelivery(
	ctx context.Context,
	aggregateID string,
	version int64,
) error {
	if err := t.exec(ctx, "INSERT INTO deliveries (aggregate_id, version) VALUES (?, ?)",
		aggregateID, version); err != nil {
		return fmt.Errorf("eventstore: enqueue delivery: %w", err)
	}
	return nil
}

func (t *tx) exec(
	ctx context.Context,
	query string,
	args ...any,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := t.tx.ExecContext(ctx, query, args...)
	return err
}

func isConstraint(
	err error,
) bool {
	var coded interface{ Code() int }
	return errors.As(err, &coded) && coded.Code()&0xff == sqliteConstraint
}
