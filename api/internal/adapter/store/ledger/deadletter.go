package ledger

import (
	"context"
	"fmt"
	"time"
)

// maxDeadLetterPayload bounds what one dead letter keeps of its hook's payload.
const maxDeadLetterPayload = 64 << 10

// deadLetterRetention is how long a dead letter is kept, whatever chat it names.
const deadLetterRetention = 30 * 24 * time.Hour

// deadLetterCountSQL is answered from the reason index alone.
const deadLetterCountSQL = `SELECT count(*) FROM ledger_dead_letters WHERE reason IN ?`

// DeadLetter is a hook the ingress could not place: who sent it, what it was,
// why it was refused, and its payload.
type DeadLetter struct {
	ChatID, RunnerID, Provider, Event, Reason string
	Payload                                   []byte
}

// DeadLetter records d, keeping at most 64 KiB of its payload.
func (s *Store) DeadLetter(ctx context.Context, d DeadLetter) error {
	payload, truncated := d.Payload, false
	if len(payload) > maxDeadLetterPayload {
		payload, truncated = payload[:maxDeadLetterPayload], true
	}
	if payload == nil {
		payload = []byte{}
	}
	err := s.db.WithContext(ctx).Exec(
		`INSERT INTO ledger_dead_letters (at, chat_id, runner_id, provider, event, reason, payload, truncated)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.clock().UnixMicro(), d.ChatID, d.RunnerID, d.Provider, d.Event, d.Reason, payload, truncated,
	).Error
	if err != nil {
		return fmt.Errorf("ledger: dead letter: %w", err)
	}
	return nil
}

// DeadLetterCount is how many dead letters were kept for one of reasons.
func (s *Store) DeadLetterCount(ctx context.Context, reasons ...string) (int, error) {
	var n int
	err := s.db.WithContext(ctx).Raw(deadLetterCountSQL, reasons).Scan(&n).Error
	if err != nil {
		return 0, fmt.Errorf("ledger: dead letter count: %w", err)
	}
	return n, nil
}

// purgeDeadLetters deletes the dead letters older than deadLetterRetention.
func (s *Store) purgeDeadLetters(ctx context.Context) error {
	cutoff := s.clock().Add(-deadLetterRetention).UnixMicro()
	if err := s.db.WithContext(ctx).Exec(`DELETE FROM ledger_dead_letters WHERE at < ?`, cutoff).Error; err != nil {
		return fmt.Errorf("purge dead letters: %w", err)
	}
	return nil
}
