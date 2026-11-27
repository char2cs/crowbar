package chatledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func Impl() migrate.Impl {
	return migrate.Impl{Apply: apply}
}

const noteBytes = 2048

var errNotAsConverted = errors.New("chatledger: the ledger is not what the conversion wrote")

var errSessionIndex = errors.New("chatledger: the session index cannot be read")

var errUnconvertible = errors.New("chatledger: chat cannot be converted")

type ledgerStore interface {
	SessionOwner(ctx context.Context, provider, sessionID string) (string, bool, error)
	Import(ctx context.Context, chatID string, rows []ledger.Row) error
	Read(ctx context.Context, chatID string, afterSeq int64, limit int) ([]ledger.Row, error)
	State(ctx context.Context, chatID string) (ledger.State, error)
	Append(ctx context.Context, chatID string, rows ...ledger.Row) ([]ledger.Row, ledger.State, error)
	ImportAfter(ctx context.Context, chatID string, head int64, rows []ledger.Row) error
}

type checkpoints interface {
	Settled(key string) bool
	Marked(key string) (migrate.Counts, bool)
	Mark(key string, n migrate.Counts) error
	Noted(key string) (migrate.Note, bool)
	Note(key string, n migrate.Note) error
}

func apply(ctx context.Context, env migrate.Env, cp *migrate.Checkpoints) (_ migrate.Counts, err error) {
	stamp, err := legacyStamp(env.Home)
	if err != nil || stamp == "" {
		return nil, err
	}
	src, err := openSource(ctx, env.Home)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, src.Close()) }()
	ids, err := src.ids(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := census(ctx, env.Home)
	if err != nil {
		return nil, err
	}
	var total migrate.Counts
	if len(ids) > 0 {
		err = withStore(ctx, env.Home, func(store *ledgerstore.Store) error {
			var err error
			total, err = convertAll(ctx, env.Log, store, cp, legacyHome{ids: ids, load: src.load, counts: counts})
			return err
		})
	}
	if err != nil {
		return total, err
	}
	owed := migrate.Counts{retryingChats: total[retryingChats]}
	if note, ok := cp.Noted(stampKey); !ok || note.Detail != stamp || note.Counts[retryingChats] != owed[retryingChats] {
		err = cp.Note(stampKey, migrate.Note{Detail: stamp, Counts: owed})
	}
	return total, err
}

func importChat(ctx context.Context, log *slog.Logger, store ledgerStore, chat LegacyChat) (Result, error) {
	res, err := convertOver(ctx, log, store, chat)
	if err != nil {
		return Result{}, err
	}
	if len(res.Skipped) > 0 {
		log.WarnContext(ctx, "chatledger: skipped records", "chat_id", chat.ChatID, "skipped", clip(strings.Join(res.Skipped, "; "), noteBytes))
	}
	if err := unlessCommittedBefore(ctx, store, chat.ChatID, res.Rows, importSafely(ctx, log, store, chat.ChatID, res.Rows)); err != nil {
		return res, err
	}
	return res, landedAsWritten(ctx, store, chat, int64(len(res.Rows)))
}

func convertOver(ctx context.Context, log *slog.Logger, store ledgerStore, chat LegacyChat) (Result, error) {
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
	return res, nil
}

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

func landedAsWritten(ctx context.Context, store ledgerStore, chat LegacyChat, rows int64) error {
	held, err := store.Read(ctx, chat.ChatID, 0, 0)
	if err != nil {
		return fmt.Errorf("chatledger: chat %s: %w", chat.ChatID, err)
	}
	st := ledger.Fold(chat.ChatID, held)
	if st.Head() != rows {
		return fmt.Errorf("%w: chat %s: it holds %d rows, the import wrote %d", errNotAsConverted, chat.ChatID, st.Head(), rows)
	}
	if err := AtRest(st, adoptedBy(chat)); err != nil {
		return fmt.Errorf("%w: chat %s: %w", errNotAsConverted, chat.ChatID, err)
	}
	return nil
}

func importSafely(ctx context.Context, log *slog.Logger, store ledgerStore, chatID string, rows []ledger.Row) (err error) {
	defer func() {
		if p := recover(); p != nil {
			log.ErrorContext(ctx, "chatledger: the import of a chat panicked", "chat_id", chatID, "panic", p, "stack", string(debug.Stack()))
			err = fmt.Errorf("%w: import %s: panic: %v", errUnconvertible, chatID, p)
		}
	}()
	return store.Import(ctx, chatID, rows)
}

func convertSafely(ctx context.Context, log *slog.Logger, chat LegacyChat, owner Owner) (res Result, err error) {
	defer func() {
		if p := recover(); p != nil {
			log.ErrorContext(ctx, "chatledger: the conversion of a chat panicked", "chat_id", chat.ChatID, "panic", p, "stack", string(debug.Stack()))
			res, err = Result{}, fmt.Errorf("chatledger: convert %s: panic: %v", chat.ChatID, p)
		}
	}()
	return ConvertChat(chat, owner)
}

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
