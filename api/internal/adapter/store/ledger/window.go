package ledger

import (
	"context"
	"fmt"
)

// WindowStart is the seq a feed of chatID's newest turns replays after: the
// first row of its newest `turns` main turns and of the turns keep names, and
// a turn with any row after that point comes whole, so no row it folds names
// a turn the feed lacks. 0 when the window reaches back to the chat's start.
func (s *Store) WindowStart(ctx context.Context, chatID string, turns int, keep []string) (int64, error) {
	var newest []string
	if err := s.db.WithContext(ctx).Raw(
		`SELECT turn_id FROM chat_ledger WHERE chat_id = ? AND kind = 'turn_started' AND scope = ''
		 ORDER BY seq DESC LIMIT ?`, chatID, turns,
	).Scan(&newest).Error; err != nil {
		return 0, fmt.Errorf("ledger: window %s: newest turns: %w", chatID, err)
	}
	if len(newest) < turns {
		return 0, nil
	}
	held := map[string]bool{}
	after := int64(-1)
	for want := append(newest, keep...); len(want) > 0; {
		first, err := s.firstOf(ctx, chatID, want)
		if err != nil {
			return 0, err
		}
		for _, id := range want {
			held[id] = true
		}
		if after < 0 || first-1 < after {
			after = first - 1
		}
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
