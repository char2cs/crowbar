package ledger

import (
	"context"
	"fmt"
	"math"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

const rowOverhead = 200

const scanRows = 4096

type WindowSpec struct{ Rows, Bytes, Upto, PreviewCap int64 }

func (s *Store) Window(ctx context.Context, chatID string, spec WindowSpec, keep []string) (int64, error) {
	upto := spec.Upto
	if upto <= 0 {
		if err := s.db.WithContext(ctx).Raw(
			`SELECT COALESCE(MAX(seq), 0) FROM chat_ledger WHERE chat_id = ?`, chatID,
		).Scan(&upto).Error; err != nil {
			return 0, fmt.Errorf("ledger: window %s: head: %w", chatID, err)
		}
	}
	after := int64(0)
	if spec.Rows > 0 {
		after = upto - spec.Rows
	}
	if spec.Bytes > 0 {
		cut, err := s.bytesCut(ctx, chatID, upto, spec)
		if err != nil {
			return 0, err
		}
		after = max(after, cut)
	}
	after = min(after, upto-1)
	if after <= 0 {
		return 0, nil
	}
	held := map[string]bool{}
	want, err := s.turnsAfter(ctx, chatID, after, upto, held)
	if err != nil {
		return 0, err
	}
	for want = append(want, keep...); len(want) > 0; {
		first, err := s.firstOf(ctx, chatID, want)
		if err != nil {
			return 0, err
		}
		for _, id := range want {
			held[id] = true
		}
		after = min(after, first-1)
		if want, err = s.turnsAfter(ctx, chatID, after, upto, held); err != nil {
			return 0, err
		}
	}
	return max(after, 0), nil
}

func (s *Store) bytesCut(ctx context.Context, chatID string, upto int64, spec WindowSpec) (int64, error) {
	floor := max(upto-scanRows, 0)
	previewCap, kinds := spec.PreviewCap, []string{}
	for _, k := range ledger.PreviewKinds() {
		kinds = append(kinds, string(k))
	}
	if previewCap <= 0 {
		previewCap, kinds = math.MaxInt64, []string{""}
	}
	var oldest *int64
	if err := s.db.WithContext(ctx).Raw(
		`SELECT MIN(seq) FROM (
			SELECT seq, SUM(CASE WHEN kind IN ? THEN MIN(length(body), ?) ELSE length(body) END + ?)
				OVER (ORDER BY seq DESC) AS size FROM chat_ledger
			WHERE chat_id = ? AND seq > ? AND seq <= ?) WHERE size <= ?`,
		kinds, previewCap, rowOverhead, chatID, floor, upto, spec.Bytes,
	).Scan(&oldest).Error; err != nil {
		return 0, fmt.Errorf("ledger: window %s: bytes: %w", chatID, err)
	}
	if oldest == nil {
		return upto - 1, nil
	}
	return *oldest - 1, nil
}

func (s *Store) firstOf(ctx context.Context, chatID string, turnIDs []string) (int64, error) {
	var first *int64
	if err := s.db.WithContext(ctx).Raw(
		`SELECT MIN(seq) FROM chat_ledger WHERE chat_id = ? AND turn_id IN ?`, chatID, turnIDs,
	).Scan(&first).Error; err != nil {
		return 0, fmt.Errorf("ledger: window %s: first row: %w", chatID, err)
	}
	if first == nil {
		return 1, nil
	}
	return *first, nil
}

func (s *Store) turnsAfter(ctx context.Context, chatID string, after, upto int64, held map[string]bool) ([]string, error) {
	var ids []string
	if err := s.db.WithContext(ctx).Raw(
		`SELECT DISTINCT turn_id FROM chat_ledger WHERE chat_id = ? AND seq > ? AND seq <= ? AND turn_id != ''`, chatID, after, upto,
	).Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("ledger: window %s: turns after %d: %w", chatID, after, err)
	}
	missing := ids[:0]
	for _, id := range ids {
		if !held[id] {
			missing = append(missing, id)
		}
	}
	return missing, nil
}
