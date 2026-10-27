package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// ErrChatNotEmpty is an import into a chat that already has rows.
var ErrChatNotEmpty = errors.New("ledger: import into a chat that has rows")

// Import writes rows, numbered 1..n and each passing Check, into chatID while
// it has none, keeping their seq and time. Heads and the session index are
// kept in the same transaction. Nothing is published.
func (s *Store) Import(ctx context.Context, chatID string, rows []ledger.Row) error {
	w := s.writer(chatID)
	w.mu.Lock()
	defer w.mu.Unlock()
	cur, err := s.load(ctx, chatID, w)
	if err != nil {
		return err
	}
	if cur.Head() > 0 {
		return fmt.Errorf("ledger: import %s: %w", chatID, ErrChatNotEmpty)
	}
	if len(rows) == 0 {
		return nil
	}
	next := ledger.State{ChatID: chatID}
	kept := make([]ledger.Row, 0, len(rows))
	for i, r := range rows {
		if r.ChatID != chatID || r.Seq != int64(i+1) {
			return fmt.Errorf("ledger: import %s: %w: row %d is %s seq %d", chatID, ledger.ErrMalformedRow, i+1, r.ChatID, r.Seq)
		}
		r.At = fromMicros(r.At.UnixMicro())
		if err := ledger.Check(next, r); err != nil {
			return fmt.Errorf("ledger: import %s: %w", chatID, err)
		}
		next.Apply(r)
		kept = append(kept, r)
	}
	if err := s.commit(ctx, chatID, kept, next); err != nil {
		return err
	}
	w.state = &next
	w.wake()
	return nil
}
