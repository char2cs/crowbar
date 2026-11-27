package ledger

import (
	"context"
	"fmt"
	"time"
)

const maxDeadLetterPayload = 64 << 10

const deadLetterRetention = 30 * 24 * time.Hour

const deadLetterCountSQL = `SELECT count(*) FROM ledger_dead_letters WHERE reason IN ?`

type DeadLetter struct {
	ChatID, RunnerID, Provider, Event, Reason string
	Payload                                   []byte
}

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

func (s *Store) DeadLetterCount(ctx context.Context, reasons ...string) (int, error) {
	var n int
	err := s.db.WithContext(ctx).Raw(deadLetterCountSQL, reasons).Scan(&n).Error
	if err != nil {
		return 0, fmt.Errorf("ledger: dead letter count: %w", err)
	}
	return n, nil
}

func (s *Store) purgeDeadLetters(ctx context.Context) error {
	cutoff := s.clock().Add(-deadLetterRetention).UnixMicro()
	if err := s.db.WithContext(ctx).Exec(`DELETE FROM ledger_dead_letters WHERE at < ?`, cutoff).Error; err != nil {
		return fmt.Errorf("purge dead letters: %w", err)
	}
	return nil
}
