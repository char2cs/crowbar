package ledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

const FileName = "chat_ledger.db"

var ErrSessionClaimed = errors.New("ledger: session is bound to another chat")

type Subscriber func(chatID string, rows []ledger.Row, st ledger.State)

type Store struct {
	db    *gorm.DB
	clock func() time.Time

	mu      sync.Mutex
	writers map[string]*chatWriter
	subs    []Subscriber
}

type chatWriter struct {
	mu      sync.Mutex
	state   *ledger.State
	changed chan struct{}
}

func Open(ctx context.Context, db *gorm.DB, clock func() time.Time) (*Store, error) {
	for _, ddl := range schema {
		if err := db.Exec(ddl).Error; err != nil {
			return nil, fmt.Errorf("ledger: open: create schema: %w", err)
		}
	}
	s := &Store{db: db, clock: clock, writers: map[string]*chatWriter{}}
	if err := s.purgeDeadLetters(ctx); err != nil {
		slog.WarnContext(ctx, "ledger: open: purge dead letters (best-effort, the next open retries)", "err", err)
	}
	return s, nil
}

func (s *Store) Subscribe(fn Subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs = append(s.subs, fn)
}

func (s *Store) Do(
	ctx context.Context,
	chatID string,
	decide func(ledger.State) ([]ledger.Row, error),
) ([]ledger.Row, ledger.State, error) {
	w := s.writer(chatID)
	w.mu.Lock()
	defer w.mu.Unlock()
	cur, err := s.load(ctx, chatID, w)
	if err != nil {
		return nil, ledger.State{}, err
	}
	rows, err := decide(cur.Clone())
	if err != nil || len(rows) == 0 {
		return nil, cur.Clone(), err
	}
	stamped, next, err := s.stamp(chatID, cur, rows)
	if err != nil {
		return nil, cur.Clone(), err
	}
	if err := s.commit(ctx, chatID, stamped, next); err != nil {
		return nil, cur.Clone(), err
	}
	w.state = &next
	w.wake()
	for _, fn := range s.subscribers() {
		fn(chatID, stamped, next)
	}
	return stamped, next.Clone(), nil
}

func (s *Store) Append(ctx context.Context, chatID string, rows ...ledger.Row) ([]ledger.Row, ledger.State, error) {
	return s.Do(ctx, chatID, func(ledger.State) ([]ledger.Row, error) { return rows, nil })
}

func (s *Store) State(ctx context.Context, chatID string) (ledger.State, error) {
	w := s.writer(chatID)
	w.mu.Lock()
	defer w.mu.Unlock()
	cur, err := s.load(ctx, chatID, w)
	if err != nil {
		return ledger.State{}, err
	}
	return cur.Clone(), nil
}

func (s *Store) View(ctx context.Context, chatID string, fn func(ledger.State) error) error {
	w := s.writer(chatID)
	w.mu.Lock()
	defer w.mu.Unlock()
	cur, err := s.load(ctx, chatID, w)
	if err != nil {
		return err
	}
	return fn(cur.Clone())
}

func (s *Store) Watch(ctx context.Context, chatID string) (ledger.State, <-chan struct{}, error) {
	w := s.writer(chatID)
	w.mu.Lock()
	defer w.mu.Unlock()
	cur, err := s.load(ctx, chatID, w)
	if err != nil {
		return ledger.State{}, nil, err
	}
	if w.changed == nil {
		w.changed = make(chan struct{})
	}
	return cur.Clone(), w.changed, nil
}

func (s *Store) Forget(ctx context.Context, chatID string) error {
	w := s.writer(chatID)
	w.mu.Lock()
	defer w.mu.Unlock()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM chat_ledger WHERE chat_id = ?`, chatID).Error; err != nil {
			return fmt.Errorf("rows: %w", err)
		}
		if err := tx.Exec(`DELETE FROM chat_ledger_heads WHERE chat_id = ?`, chatID).Error; err != nil {
			return fmt.Errorf("head: %w", err)
		}
		if err := tx.Exec(`DELETE FROM chat_ledger_sessions WHERE chat_id = ?`, chatID).Error; err != nil {
			return fmt.Errorf("sessions: %w", err)
		}
		if err := tx.Exec(`DELETE FROM chat_ledger_relays WHERE chat_id = ?`, chatID).Error; err != nil {
			return fmt.Errorf("relays: %w", err)
		}
		if err := tx.Exec(`DELETE FROM ledger_dead_letters WHERE chat_id = ?`, chatID).Error; err != nil {
			return fmt.Errorf("dead letters: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("ledger: forget %s: %w", chatID, err)
	}
	w.state = nil
	w.wake()
	return nil
}

func (w *chatWriter) wake() {
	if w.changed != nil {
		close(w.changed)
		w.changed = nil
	}
}

func (s *Store) writer(chatID string) *chatWriter {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.writers[chatID]
	if !ok {
		w = &chatWriter{}
		s.writers[chatID] = w
	}
	return w
}

func (s *Store) subscribers() []Subscriber {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.subs)
}

func (s *Store) load(ctx context.Context, chatID string, w *chatWriter) (ledger.State, error) {
	if w.state != nil {
		return *w.state, nil
	}
	rows, err := s.Read(ctx, chatID, 0, 0)
	if err != nil {
		return ledger.State{}, err
	}
	st := ledger.Fold(chatID, rows)
	w.state = &st
	return st, nil
}

func (s *Store) stamp(chatID string, cur ledger.State, rows []ledger.Row) ([]ledger.Row, ledger.State, error) {
	next := cur.Clone()
	stamped := make([]ledger.Row, 0, len(rows))
	for _, r := range rows {
		r.ChatID, r.Seq, r.At = chatID, next.Head()+1, fromMicros(s.clock().UnixMicro())
		if err := ledger.Check(next, r); err != nil {
			return nil, cur, fmt.Errorf("ledger: append to %s: %w", chatID, err)
		}
		next.Apply(r)
		stamped = append(stamped, r)
	}
	return stamped, next, nil
}

func (s *Store) commit(ctx context.Context, chatID string, rows []ledger.Row, next ledger.State) error {
	open := 0
	if next.HasOpenWork() {
		open = 1
	}
	movedAway := movedAway(rows)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, r := range rows {
			if err := tx.Exec(
				`INSERT INTO chat_ledger (chat_id, seq, kind, turn_id, scope, at, body) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				r.ChatID, r.Seq, string(r.Kind), r.TurnID, r.Scope, r.At.UnixMicro(), []byte(r.Body),
			).Error; err != nil {
				return fmt.Errorf("row %d: %w", r.Seq, err)
			}
			if err := indexSession(tx, chatID, r, movedAway); err != nil {
				return fmt.Errorf("row %d: %w", r.Seq, err)
			}
		}
		if err := tx.Exec(
			`INSERT INTO chat_ledger_heads (chat_id, last_seq, open) VALUES (?, ?, ?)
			 ON CONFLICT(chat_id) DO UPDATE SET last_seq = excluded.last_seq, open = excluded.open`,
			chatID, next.Head(), open,
		).Error; err != nil {
			return fmt.Errorf("head: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("ledger: append to %s: %w", chatID, err)
	}
	return nil
}

type sessionKey struct{ provider, sessionID string }

func movedAway(rows []ledger.Row) map[sessionKey]bool {
	out := map[sessionKey]bool{}
	for _, r := range rows {
		if body, err := ledger.DecodeBody[ledger.SessionLost](r); err == nil && r.Kind == ledger.KindSessionLost && body.MovedTo != "" {
			out[sessionKey{body.Provider, body.SessionID}] = true
		}
	}
	return out
}

func indexSession(tx *gorm.DB, chatID string, r ledger.Row, movedAway map[sessionKey]bool) error {
	if r.Kind == ledger.KindSessionLost {
		body, err := ledger.DecodeBody[ledger.SessionLost](r)
		if err != nil || body.MovedTo == "" {
			return err
		}
		return passSession(tx, sessionKey{body.Provider, body.SessionID}, chatID, body.MovedTo)
	}
	if r.Kind != ledger.KindSessionBound {
		return nil
	}
	body, err := ledger.DecodeBody[ledger.SessionBound](r)
	if err != nil {
		return err
	}
	key := sessionKey{body.Provider, body.SessionID}
	if body.Cause != ledger.CauseMoved && movedAway[key] {
		return nil
	}
	if body.Cause == ledger.CauseMoved {
		if err := passSession(tx, key, body.From, chatID); err != nil {
			return err
		}
	}
	if err := claimSession(tx, chatID, key); err != nil {
		return err
	}
	return indexRelay(tx, chatID, body)
}

func passSession(tx *gorm.DB, key sessionKey, from, to string) error {
	err := tx.Exec(`UPDATE chat_ledger_sessions SET chat_id = ? WHERE provider = ? AND session_id = ? AND chat_id = ?`,
		to, key.provider, key.sessionID, from).Error
	if err != nil {
		return fmt.Errorf("session index: pass on: %w", err)
	}
	return nil
}

func claimSession(tx *gorm.DB, chatID string, key sessionKey) error {
	claim := tx.Exec(`INSERT INTO chat_ledger_sessions (provider, session_id, chat_id) VALUES (?, ?, ?)
		ON CONFLICT(provider, session_id) DO NOTHING`, key.provider, key.sessionID, chatID)
	if claim.Error != nil {
		return fmt.Errorf("session index: %w", claim.Error)
	}
	if claim.RowsAffected == 1 {
		return nil
	}
	var owners []string
	if err := tx.Raw(`SELECT chat_id FROM chat_ledger_sessions WHERE provider = ? AND session_id = ?`,
		key.provider, key.sessionID).Scan(&owners).Error; err != nil {
		return fmt.Errorf("session owner: %w", err)
	}
	if len(owners) != 1 || owners[0] != chatID {
		return fmt.Errorf("%s %s: %w", key.provider, key.sessionID, ErrSessionClaimed)
	}
	return nil
}
