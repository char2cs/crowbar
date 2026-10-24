// Package ledger stores each chat's ledger rows in an append-only SQLite
// table, one writer per chat. Import it as ledgerstore.
package ledger

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// FileName is the ledger database under the store directory.
const FileName = "chat_ledger.db"

// ErrSessionClaimed is a session_bound row naming a session another chat owns.
var ErrSessionClaimed = errors.New("ledger: session is bound to another chat")

// Subscriber is called after commit, under the chat's writer, in seq order.
// It must not block: hand work to a channel or do it in O(1). st is a read-only
// view of the writer's cache: never mutate it or Apply to it.
type Subscriber func(chatID string, rows []ledger.Row, st ledger.State)

// Store is the chat ledger. Each chat has a single writer that owns its seqs
// and its cached State.
type Store struct {
	db    *gorm.DB
	clock func() time.Time

	mu      sync.Mutex
	writers map[string]*chatWriter
	subs    []Subscriber
}

// chatWriter serialises one chat's appends; state is nil until first touch.
// changed, once watched, is closed by the next commit or Forget.
type chatWriter struct {
	mu      sync.Mutex
	state   *ledger.State
	changed chan struct{}
}

// Open creates the ledger tables on db if they are missing.
func Open(db *gorm.DB, clock func() time.Time) (*Store, error) {
	for _, ddl := range schema {
		if err := db.Exec(ddl).Error; err != nil {
			return nil, fmt.Errorf("ledger: open: create schema: %w", err)
		}
	}
	return &Store{db: db, clock: clock, writers: map[string]*chatWriter{}}, nil
}

// Subscribe adds fn to the subscribers every later commit is published to.
func (s *Store) Subscribe(fn Subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs = append(s.subs, fn)
}

// Do runs decide under chatID's writer against the chat's current State and
// appends the rows it returns: seqs assigned, each row checked, rows and head
// committed in one transaction, the State advanced, subscribers called. It
// returns the committed rows and the State after them. No rows commit nothing.
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

// Append is Do with a decision that always appends rows.
func (s *Store) Append(ctx context.Context, chatID string, rows ...ledger.Row) ([]ledger.Row, ledger.State, error) {
	return s.Do(ctx, chatID, func(ledger.State) ([]ledger.Row, error) { return rows, nil })
}

// State is chatID's current State.
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

// View runs fn on chatID's State under the chat's writer, so no commit lands
// while it runs. fn may Read the chat's rows; it must never Do, Append or View.
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

// Watch is chatID's current State and a channel closed by the chat's next
// commit, read together so no commit between them can be missed.
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

// Forget erases chatID's rows, head, sessions and dead letters, as a purged chat
// must leave nothing.
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

// wake closes the watchers' signal. The caller holds w.mu.
func (w *chatWriter) wake() {
	if w.changed != nil {
		close(w.changed)
		w.changed = nil
	}
}

// writer is chatID's writer. Entries are never removed: a waiter holding one
// must contend with every later caller on the same mutex.
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

// load folds chatID's rows into w on first touch. The caller holds w.mu.
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

// stamp numbers and dates rows after cur and checks each against the State
// the rows before it leave.
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
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, r := range rows {
			if err := tx.Exec(
				`INSERT INTO chat_ledger (chat_id, seq, kind, turn_id, scope, at, body) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				r.ChatID, r.Seq, string(r.Kind), r.TurnID, r.Scope, r.At.UnixMicro(), []byte(r.Body),
			).Error; err != nil {
				return fmt.Errorf("row %d: %w", r.Seq, err)
			}
			if err := claimSession(tx, chatID, r); err != nil {
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

// claimSession indexes a session_bound row's session under chatID, refusing a
// session another chat owns. The insert is the claim: only when it took no row
// is the session already someone's, and then only its owner may rebind it.
func claimSession(tx *gorm.DB, chatID string, r ledger.Row) error {
	if r.Kind != ledger.KindSessionBound {
		return nil
	}
	body, err := ledger.DecodeBody[ledger.SessionBound](r)
	if err != nil {
		return err
	}
	claim := tx.Exec(`INSERT INTO chat_ledger_sessions (provider, session_id, chat_id) VALUES (?, ?, ?)
		ON CONFLICT(provider, session_id) DO NOTHING`, body.Provider, body.SessionID, chatID)
	if claim.Error != nil {
		return fmt.Errorf("session index: %w", claim.Error)
	}
	if claim.RowsAffected == 1 {
		return nil
	}
	var owners []string
	if err := tx.Raw(`SELECT chat_id FROM chat_ledger_sessions WHERE provider = ? AND session_id = ?`,
		body.Provider, body.SessionID).Scan(&owners).Error; err != nil {
		return fmt.Errorf("session owner: %w", err)
	}
	if len(owners) != 1 || owners[0] != chatID {
		return fmt.Errorf("%s %s: %w", body.Provider, body.SessionID, ErrSessionClaimed)
	}
	return nil
}

// maxDeadLetterPayload bounds what one dead letter keeps of its hook's payload.
const maxDeadLetterPayload = 64 << 10

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
	err := s.db.WithContext(ctx).Raw(`SELECT count(*) FROM ledger_dead_letters WHERE reason IN ?`, reasons).Scan(&n).Error
	if err != nil {
		return 0, fmt.Errorf("ledger: dead letter count: %w", err)
	}
	return n, nil
}
