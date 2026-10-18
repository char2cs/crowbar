package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// record is one chat_ledger row as the table holds it.
type record struct {
	ChatID string
	Seq    int64
	Kind   string
	TurnID string
	Scope  string
	At     int64
	Body   []byte
}

// Read returns chatID's rows after afterSeq in seq order, at most limit of
// them; limit <= 0 is all of them.
func (s *Store) Read(ctx context.Context, chatID string, afterSeq int64, limit int) ([]ledger.Row, error) {
	if limit <= 0 {
		limit = -1 // SQLite: no limit
	}
	var recs []record
	err := s.db.WithContext(ctx).Raw(
		`SELECT chat_id, seq, kind, turn_id, scope, at, body FROM chat_ledger
		 WHERE chat_id = ? AND seq > ? ORDER BY seq LIMIT ?`, chatID, afterSeq, limit,
	).Scan(&recs).Error
	if err != nil {
		return nil, fmt.Errorf("ledger: read %s: %w", chatID, err)
	}
	rows := make([]ledger.Row, 0, len(recs))
	for _, r := range recs {
		rows = append(rows, ledger.Row{
			ChatID: r.ChatID, Seq: r.Seq, Kind: ledger.Kind(r.Kind), TurnID: r.TurnID,
			Scope: r.Scope, At: fromMicros(r.At), Body: r.Body,
		})
	}
	return rows, nil
}

// OpenChats lists the chats whose last append left work unfinished.
func (s *Store) OpenChats(ctx context.Context) ([]string, error) {
	var ids []string
	err := s.db.WithContext(ctx).Raw(
		`SELECT chat_id FROM chat_ledger_heads WHERE open = 1 ORDER BY chat_id`,
	).Scan(&ids).Error
	if err != nil {
		return nil, fmt.Errorf("ledger: open chats: %w", err)
	}
	return ids, nil
}

// SessionOwner answers which chat a (provider, session) is bound to.
func (s *Store) SessionOwner(ctx context.Context, provider, sessionID string) (chatID string, ok bool, err error) {
	var owners []string
	err = s.db.WithContext(ctx).Raw(
		`SELECT chat_id FROM chat_ledger_sessions WHERE provider = ? AND session_id = ?`, provider, sessionID,
	).Scan(&owners).Error
	if err != nil {
		return "", false, fmt.Errorf("ledger: session owner %s %s: %w", provider, sessionID, err)
	}
	if len(owners) == 0 {
		return "", false, nil
	}
	return owners[0], true, nil
}

// fromMicros is the time a stored at column names, as the store stamped it.
func fromMicros(us int64) time.Time { return time.UnixMicro(us).UTC() }
