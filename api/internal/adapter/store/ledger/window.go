package ledger

import (
	"context"
	"fmt"
)

// WindowStart is the seq a feed of chatID's newest `rows` rows replays after,
// widened so no turn is cut: a turn with any row after that point comes whole,
// and so does every turn keep names, so no row the client folds names a turn it
// lacks. 0 when the window reaches back to the chat's start.
func (s *Store) WindowStart(ctx context.Context, chatID string, rows int64, keep []string) (int64, error) {
	var head int64
	if err := s.db.WithContext(ctx).Raw(
		`SELECT COALESCE(MAX(seq), 0) FROM chat_ledger WHERE chat_id = ?`, chatID,
	).Scan(&head).Error; err != nil {
		return 0, fmt.Errorf("ledger: window %s: head: %w", chatID, err)
	}
	after := head - rows
	if after <= 0 {
		return 0, nil
	}
	held := map[string]bool{}
	want, err := s.turnsAfter(ctx, chatID, after, held)
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
		if want, err = s.turnsAfter(ctx, chatID, after, held); err != nil {
			return 0, err
		}
	}
	return max(after, 0), nil
}

// firstOf is the seq of the first row of any of turnIDs.
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

// turnsAfter are the turns with a row after `after` that held lacks.
func (s *Store) turnsAfter(ctx context.Context, chatID string, after int64, held map[string]bool) ([]string, error) {
	var ids []string
	if err := s.db.WithContext(ctx).Raw(
		`SELECT DISTINCT turn_id FROM chat_ledger WHERE chat_id = ? AND seq > ? AND turn_id != ''`, chatID, after,
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
