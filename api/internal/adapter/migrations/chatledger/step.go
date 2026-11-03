package chatledger

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// Impl converts each legacy chat into the ledger once, checkpointed per chat.
// Each import is checked as it lands, before its checkpoint, so a checkpoint
// means checked: rows normal use adds later are never measured against it.
func Impl() migrate.Impl {
	return migrate.Impl{Apply: apply}
}

// noteBytes bounds what one chat's skip puts in the log and the run ledger.
const noteBytes = 2048

// errNotAsConverted is a committed import whose chat does not hold what was
// converted: a retry cannot change rows already there, so it is given up.
var errNotAsConverted = errors.New("chatledger: the ledger is not what the conversion wrote")

// errSessionIndex is a session index that cannot be read: read as "nobody owns
// it", a chat could take a session another chat holds, so the step stops.
var errSessionIndex = errors.New("chatledger: the session index cannot be read")

// errUnconvertible is a chat whose legacy records do not restate as rows at
// rest: it is skipped and named, its legacy files kept for a later build.
var errUnconvertible = errors.New("chatledger: chat cannot be converted")

// ledgerStore is the part of the chat ledger the conversion writes through.
type ledgerStore interface {
	SessionOwner(ctx context.Context, provider, sessionID string) (string, bool, error)
	Import(ctx context.Context, chatID string, rows []ledger.Row) error
	Read(ctx context.Context, chatID string, afterSeq int64, limit int) ([]ledger.Row, error)
	State(ctx context.Context, chatID string) (ledger.State, error)
	Append(ctx context.Context, chatID string, rows ...ledger.Row) ([]ledger.Row, ledger.State, error)
}

// checkpoints is the part of *migrate.Checkpoints the conversion uses.
type checkpoints interface {
	Settled(key string) bool
	Marked(key string) (migrate.Counts, bool)
	Mark(key string, n migrate.Counts) error
	Noted(key string) (migrate.Note, bool)
	Note(key string, n migrate.Note) error
}

func apply(ctx context.Context, env migrate.Env, cp *migrate.Checkpoints) (migrate.Counts, error) {
	stamp, err := legacyStamp(env.Home)
	if err != nil || stamp == "" {
		return nil, err
	}
	chats, err := ReadLegacy(ctx, env.Home)
	if err != nil {
		return nil, err
	}
	counts, err := census(ctx, env.Home)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(chats, func(a, b LegacyChat) int {
		return cmp.Or(oldest(a).Compare(oldest(b)), cmp.Compare(a.ChatID, b.ChatID))
	})
	var total migrate.Counts
	if len(chats) > 0 {
		err = withStore(ctx, env.Home, func(store *ledgerstore.Store) error {
			var err error
			total, err = convertAll(ctx, env.Log, store, cp, legacyHome{chats, counts})
			return err
		})
	}
	if err != nil {
		return total, err
	}
	if note, ok := cp.Noted(stampKey); !ok || note.Detail != stamp {
		err = cp.Note(stampKey, migrate.Note{Detail: stamp})
	}
	return total, err
}

// importChat converts chat, imports it and checks the chat holds what it
// wrote. A chat already holding exactly those rows is an import an
// interrupted attempt committed and never marked.
func importChat(ctx context.Context, log *slog.Logger, store ledgerStore, chat LegacyChat) (Result, error) {
	var ownerErr error
	res, err := convertSafely(ctx, log, chat, func(provider, sessionID string) (string, bool) {
		chatID, ok, err := store.SessionOwner(ctx, provider, sessionID)
		ownerErr = errors.Join(ownerErr, err)
		return chatID, ok
	})
	if ownerErr != nil {
		return Result{}, fmt.Errorf("%w: chat %s: %w", errSessionIndex, chat.ChatID, ownerErr)
	}
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", errUnconvertible, err)
	}
	if len(res.Skipped) > 0 {
		log.WarnContext(ctx, "chatledger: skipped records", "chat_id", chat.ChatID, "skipped", clip(strings.Join(res.Skipped, "; "), noteBytes))
	}
	if err := unlessCommittedBefore(ctx, store, chat.ChatID, res.Rows, importSafely(ctx, log, store, chat.ChatID, res.Rows)); err != nil {
		return res, err
	}
	return res, landedAsWritten(ctx, store, chat, int64(len(res.Rows)))
}

// unlessCommittedBefore is the import's err, but nil when it was refused for
// rows already there that are exactly the rows it would write: an interrupted
// attempt's import, never marked.
func unlessCommittedBefore(ctx context.Context, store ledgerStore, chatID string, rows []ledger.Row, err error) error {
	if !errors.Is(err, ledgerstore.ErrChatNotEmpty) {
		return err
	}
	held, readErr := store.Read(ctx, chatID, 0, 0)
	if readErr != nil {
		return fmt.Errorf("chatledger: chat %s: %w", chatID, readErr)
	}
	if reflect.DeepEqual(held, rows) {
		return nil
	}
	return err
}

// landedAsWritten checks, right after the import, that the chat holds the rows
// it wrote and is at rest but for the runners the boot reconcile ends.
func landedAsWritten(ctx context.Context, store ledgerStore, chat LegacyChat, rows int64) error {
	st, err := store.State(ctx, chat.ChatID)
	if err != nil {
		return fmt.Errorf("chatledger: chat %s: %w", chat.ChatID, err)
	}
	if st.Head() != rows {
		return fmt.Errorf("%w: chat %s: it holds %d rows, the import wrote %d", errNotAsConverted, chat.ChatID, st.Head(), rows)
	}
	if err := AtRest(st, adoptedBy(chat)); err != nil {
		return fmt.Errorf("%w: chat %s: %w", errNotAsConverted, chat.ChatID, err)
	}
	return nil
}

// importSafely is the store's Import with a panic made the chat's error, as
// convertSafely does for the conversion; the store's transaction rolls back.
func importSafely(ctx context.Context, log *slog.Logger, store ledgerStore, chatID string, rows []ledger.Row) (err error) {
	defer func() {
		if p := recover(); p != nil {
			log.ErrorContext(ctx, "chatledger: the import of a chat panicked", "chat_id", chatID, "panic", p, "stack", string(debug.Stack()))
			err = fmt.Errorf("%w: import %s: panic: %v", errUnconvertible, chatID, p)
		}
	}()
	return store.Import(ctx, chatID, rows)
}

// convertSafely is ConvertChat with a panic made the chat's error: one chat's
// damaged records must not stop every boot.
func convertSafely(ctx context.Context, log *slog.Logger, chat LegacyChat, owner Owner) (res Result, err error) {
	defer func() {
		if p := recover(); p != nil {
			log.ErrorContext(ctx, "chatledger: the conversion of a chat panicked", "chat_id", chat.ChatID, "panic", p, "stack", string(debug.Stack()))
			res, err = Result{}, fmt.Errorf("chatledger: convert %s: panic: %v", chat.ChatID, p)
		}
	}()
	return ConvertChat(chat, owner)
}

// withStore runs fn over the home's chat ledger, closed before it returns.
func withStore(ctx context.Context, h home.Home, fn func(*ledgerstore.Store) error) error {
	if err := os.MkdirAll(h.Store(), 0o700); err != nil {
		return fmt.Errorf("chatledger: open the ledger: %w", err)
	}
	db, err := storesqlite.OpenDB(filepath.Join(h.Store(), ledgerstore.FileName))
	if err != nil {
		return fmt.Errorf("chatledger: open the ledger: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("chatledger: open the ledger: %w", err)
	}
	store, err := ledgerstore.Open(ctx, db, time.Now)
	if err == nil {
		err = fn(store)
	}
	return errors.Join(err, sqlDB.Close())
}

func adoptedBy(chat LegacyChat) map[string]bool {
	out := map[string]bool{}
	for _, r := range chat.Runners {
		out[r.ID] = true
	}
	return out
}

// oldest is the time of chat's oldest legacy record.
func oldest(chat LegacyChat) time.Time {
	var ts []time.Time
	for _, c := range chat.Conversations {
		ts = append(ts, c.FirstSeenAt)
	}
	for _, t := range chat.Turns {
		ts = append(ts, t.StartedAt)
	}
	for _, p := range chat.Prompts {
		ts = append(ts, p.CreatedAt)
	}
	return earliest(ts...)
}
