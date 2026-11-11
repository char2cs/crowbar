package chatledger

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// source is the legacy files held open for one conversion. It names every
// legacy chat oldest first, then reads one chat at a time, so a home's whole
// history is never in memory at once.
type source struct {
	h       home.Home
	dbs     map[string]*sql.DB
	tables  []legacyTable
	sel     map[string]string
	byKey   map[string]bool
	runners map[string][]LegacyRunner
	written time.Time
}

// openSource opens the legacy databases read-only; a missing one is no rows.
func openSource(ctx context.Context, h home.Home) (*source, error) {
	s := &source{
		h: h, dbs: map[string]*sql.DB{}, sel: map[string]string{}, byKey: map[string]bool{}, runners: map[string][]LegacyRunner{},
	}
	if err := s.open(ctx); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	return s, nil
}

func (s *source) open(ctx context.Context) error {
	for _, name := range []string{RunnerDB, ActivityDB} {
		db, ok, err := openLegacy(filepath.Join(s.h.Store(), name))
		if err != nil {
			return err
		}
		if ok {
			s.dbs[name] = db
		}
	}
	for _, t := range legacyTables {
		if err := s.addTable(ctx, t); err != nil {
			return err
		}
	}
	runners, err := LiveRunners(ctx, s.h)
	if err != nil {
		return err
	}
	for _, run := range runners {
		s.runners[run.ChatID] = append(s.runners[run.ChatID], run)
	}
	s.written, err = modTime(filepath.Join(s.h.Store(), RunnerDB))
	return err
}

// addTable takes t on, as the columns its table has, when its database holds
// it, and finds out whether a chat's rows can be looked up by the key the old
// daemon gave each row.
func (s *source) addTable(ctx context.Context, t legacyTable) error {
	db := s.dbs[t.db]
	if db == nil {
		return nil
	}
	has, err := tableColumns(ctx, db, t.name)
	if err != nil || len(has) == 0 {
		return err
	}
	s.tables = append(s.tables, t)
	s.sel[t.name] = t.selectList(has)
	keyed, err := prefixKeyed(ctx, db, t.name)
	s.byKey[t.name] = keyed
	return err
}

// prefixKeyed reports whether every row of table is keyed <chat id>\x00<id>.
// Only then does a range on the key find all of a chat's rows; a table any row
// of which breaks the rule is read by chat id instead, which scans it.
func prefixKeyed(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var hasKey int
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM pragma_table_info(?) WHERE name = 'key'", table).Scan(&hasKey); err != nil {
		return false, fmt.Errorf("chatledger: %s: %w", table, err)
	}
	if hasKey == 0 {
		return false, nil
	}
	var stray int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table+` WHERE COALESCE(chat_id, '') != ''
		AND NOT (key IS NOT NULL
			AND substr(CAST(key AS BLOB), 1, length(CAST(chat_id AS BLOB))) = CAST(chat_id AS BLOB)
			AND substr(CAST(key AS BLOB), length(CAST(chat_id AS BLOB)) + 1, 1) = x'00')`).Scan(&stray)
	if err != nil {
		return false, fmt.Errorf("chatledger: %s: %w", table, err)
	}
	return stray == 0, nil
}

// Close closes the legacy databases.
func (s *source) Close() error {
	var errs []error
	for _, db := range s.dbs {
		errs = append(errs, db.Close())
	}
	return errors.Join(errs...)
}

// chatKey is what orders a chat before it is read: its oldest record, and
// whether a conversation of it has no time, which dated gives the files' own.
type chatKey struct {
	oldest  time.Time
	undated bool
}

// ids lists every legacy chat, oldest first, as ConvertChat must meet them: an
// older chat claims a session before a newer one. It keeps one key per chat.
func (s *source) ids(ctx context.Context) ([]string, error) {
	keys := map[string]*chatKey{}
	fold := func(r *reader) {
		for id, chat := range r.chats {
			k := keys[id]
			if k == nil {
				k = &chatKey{}
				keys[id] = k
			}
			k.oldest = earliest(k.oldest, oldest(*chat))
			k.undated = k.undated || slices.ContainsFunc(chat.Conversations, func(c LegacyConversation) bool {
				return c.FirstSeenAt.IsZero() && c.LastActiveAt.IsZero()
			})
		}
	}
	for _, t := range s.tables {
		err := s.query(ctx, t, "COALESCE(chat_id, '') != ''", nil, func(rows *sql.Rows) error {
			r := &reader{chats: map[string]*LegacyChat{}}
			if err := t.scan(r, rows); err != nil {
				return err
			}
			fold(r)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if err := s.journals(fold); err != nil {
		return nil, err
	}
	for id := range s.runners {
		fold(&reader{chats: map[string]*LegacyChat{id: {ChatID: id}}})
	}
	at := func(id string) time.Time {
		if k := keys[id]; !k.oldest.IsZero() || !k.undated {
			return k.oldest
		}
		return s.written
	}
	ids := make([]string, 0, len(keys))
	for id := range keys {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int { return cmp.Or(at(a).Compare(at(b)), cmp.Compare(a, b)) })
	return ids, nil
}

// journals folds each chat's prompt journal, one chat at a time.
func (s *source) journals(fold func(*reader)) error {
	chatsDir := filepath.Join(s.h.Root(), "chats")
	dirs, err := readDir(chatsDir)
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		r := &reader{chats: map[string]*LegacyChat{}}
		if err := r.journal(chatsDir, d.Name()); err != nil {
			return err
		}
		fold(r)
	}
	return nil
}

// load reads one chat's records.
func (s *source) load(ctx context.Context, id string) (LegacyChat, error) {
	r := &reader{chats: map[string]*LegacyChat{}}
	r.chat(id)
	for _, t := range s.tables {
		where, args := "chat_id = ?", []any{id}
		if s.byKey[t.name] {
			where, args = where+" AND key >= ? AND key < ?", append(args, id+"\x00", id+"\x01")
		}
		err := s.query(ctx, t, where+" ORDER BY "+t.order, args, func(rows *sql.Rows) error { return t.scan(r, rows) })
		if err != nil {
			return LegacyChat{}, err
		}
	}
	r.chats[id].Runners = append(r.chats[id].Runners, s.runners[id]...)
	chatsDir := filepath.Join(s.h.Root(), "chats")
	if isDir(filepath.Join(chatsDir, id)) {
		if err := r.journal(chatsDir, id); err != nil {
			return LegacyChat{}, err
		}
	}
	return dated(*r.chats[id], s.written), nil
}

// query runs SELECT t's columns FROM t WHERE where, handing each row to scan.
func (s *source) query(
	ctx context.Context, t legacyTable, where string, args []any, scan func(*sql.Rows) error,
) (err error) {
	rows, err := s.dbs[t.db].QueryContext(ctx, "SELECT "+s.sel[t.name]+" FROM "+t.name+" WHERE "+where, args...)
	if err != nil {
		return fmt.Errorf("chatledger: %s: %w", t.name, err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return fmt.Errorf("chatledger: %s: %w", t.name, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("chatledger: %s: %w", t.name, err)
	}
	return nil
}

// isDir reports whether path is a directory itself, not a link to one, as a
// listing of its parent would say.
func isDir(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}
