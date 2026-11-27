package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

var ErrChatNotEmpty = errors.New("ledger: import into a chat that has rows")

var ErrHeadMoved = errors.New("ledger: import after a head the chat has moved from")

func (s *Store) Import(ctx context.Context, chatID string, rows []ledger.Row) error {
	return s.importAt(ctx, chatID, 0, rows, ErrChatNotEmpty)
}

func (s *Store) ImportAfter(ctx context.Context, chatID string, head int64, rows []ledger.Row) error {
	return s.importAt(ctx, chatID, head, rows, ErrHeadMoved)
}

func (s *Store) importAt(ctx context.Context, chatID string, head int64, rows []ledger.Row, moved error) error {
	w := s.writer(chatID)
	w.mu.Lock()
	defer w.mu.Unlock()
	cur, err := s.load(ctx, chatID, w)
	if err != nil {
		return err
	}
	if cur.Head() != head {
		return fmt.Errorf("ledger: import %s: %w", chatID, moved)
	}
	if len(rows) == 0 {
		return nil
	}
	next := cur.Clone()
	kept := make([]ledger.Row, 0, len(rows))
	for i, r := range rows {
		if r.ChatID != chatID || r.Seq != head+int64(i+1) {
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
	w.state = nil
	w.wake()
	return nil
}
