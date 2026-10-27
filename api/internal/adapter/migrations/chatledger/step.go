package chatledger

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"time"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// Plan has one conversion of unknown size wherever the old daemon left a
// legacy database or prompt journal; it only stats files.
func Plan(_ context.Context, env migrate.Env) (migrate.Plan, error) {
	found, err := hasLegacy(env.Home)
	if err != nil || !found {
		return migrate.Plan{}, err
	}
	return migrate.Plan{
		Actions:       []migrate.Action{{Kind: migrate.Convert, Subject: "legacy chat history → chat ledger", Count: migrate.Unknown}},
		RewritesState: true,
	}, nil
}

func hasLegacy(h home.Home) (bool, error) {
	for _, db := range []string{ActivityDB, RunnerDB} {
		_, err := os.Stat(filepath.Join(h.Store(), db))
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return false, fmt.Errorf("chatledger: plan: %w", err)
		}
	}
	journals, err := filepath.Glob(filepath.Join(h.Root(), "chats", "*", PromptsDir))
	if err != nil {
		return false, fmt.Errorf("chatledger: plan: %w", err)
	}
	return len(journals) > 0, nil
}

// Impl converts each legacy chat into the ledger once, checkpointed per chat,
// and verifies every chat it converted is at rest.
func Impl() migrate.Impl {
	// Verify checks what Apply converted: the engine runs Apply first on every
	// attempt, and Apply also fills in the chats earlier attempts marked done.
	converted := map[string]map[string]bool{} // chat → the runners it is left live on
	return migrate.Impl{
		Apply: func(ctx context.Context, env migrate.Env, cp *migrate.Checkpoints) (migrate.Counts, error) {
			return apply(ctx, env, cp, converted)
		},
		Verify: func(ctx context.Context, env migrate.Env) error {
			return verify(ctx, env.Home, converted)
		},
	}
}

// errUnconvertible is a chat whose legacy records do not restate as rows at
// rest: it is skipped and named, its legacy files kept for a later build.
var errUnconvertible = errors.New("chatledger: chat cannot be converted")

// ledgerStore is the part of the chat ledger the conversion writes through.
type ledgerStore interface {
	SessionOwner(ctx context.Context, provider, sessionID string) (string, bool, error)
	Import(ctx context.Context, chatID string, rows []ledger.Row) error
	Read(ctx context.Context, chatID string, afterSeq int64, limit int) ([]ledger.Row, error)
}

// checkpoints is the part of *migrate.Checkpoints the conversion uses.
type checkpoints interface {
	Done(key string) bool
	Mark(key string, n migrate.Counts) error
}

func apply(ctx context.Context, env migrate.Env, cp *migrate.Checkpoints, converted map[string]map[string]bool) (migrate.Counts, error) {
	chats, err := ReadLegacy(ctx, env.Home)
	if err != nil || len(chats) == 0 {
		return nil, err
	}
	slices.SortStableFunc(chats, func(a, b LegacyChat) int {
		return cmp.Or(oldest(a).Compare(oldest(b)), cmp.Compare(a.ChatID, b.ChatID))
	})
	var counts migrate.Counts
	err = withStore(env.Home, func(store *ledgerstore.Store) error {
		var err error
		counts, err = convertAll(ctx, env.Log, store, cp, chats, converted)
		return err
	})
	return counts, err
}

// convertAll imports each chat not yet marked, in order. A chat whose ledger
// already has rows, or whose records cannot be converted, is skipped and
// logged with why; a store failure stops the step.
func convertAll(
	ctx context.Context, log *slog.Logger, store ledgerStore, cp checkpoints,
	chats []LegacyChat, converted map[string]map[string]bool,
) (migrate.Counts, error) {
	unmarked := migrate.Counts{}
	for _, chat := range chats {
		if err := ctx.Err(); err != nil {
			return nonZero(unmarked), err
		}
		if cp.Done(chat.ChatID) {
			converted[chat.ChatID] = adoptedBy(chat)
			continue
		}
		res, err := importChat(ctx, store, chat)
		if len(res.Skipped) > 0 {
			log.WarnContext(ctx, "chatledger: skipped records", "chat_id", chat.ChatID, "skipped", res.Skipped)
		}
		switch {
		case errors.Is(err, ledgerstore.ErrChatNotEmpty):
			log.WarnContext(ctx, "chatledger: skipped a chat whose ledger already holds rows", "chat_id", chat.ChatID)
			unmarked.Add(migrate.Counts{"skipped_chats": 1})
			continue
		case errors.Is(err, errUnconvertible):
			log.WarnContext(ctx, "chatledger: skipped a chat that cannot be converted", "chat_id", chat.ChatID, "err", err)
			unmarked.Add(migrate.Counts{"skipped_rows": 1})
			continue
		case err != nil:
			return nonZero(unmarked), err
		}
		counts := migrate.Counts{"rows": int64(len(res.Rows)), "skipped_rows": int64(len(res.Skipped))}
		if err := cp.Mark(chat.ChatID, counts); err != nil {
			return nonZero(unmarked), fmt.Errorf("chatledger: chat %s: %w", chat.ChatID, err)
		}
		converted[chat.ChatID] = adoptedBy(chat)
	}
	return nonZero(unmarked), nil
}

func nonZero(c migrate.Counts) migrate.Counts {
	if len(c) == 0 {
		return nil
	}
	return c
}

// importChat converts chat and imports it. A chat already holding exactly
// those rows is an import an interrupted attempt committed and never marked.
func importChat(ctx context.Context, store ledgerStore, chat LegacyChat) (Result, error) {
	var ownerErr error
	res, err := ConvertChat(chat, func(provider, sessionID string) (string, bool) {
		chatID, ok, err := store.SessionOwner(ctx, provider, sessionID)
		ownerErr = errors.Join(ownerErr, err)
		return chatID, ok
	})
	if ownerErr != nil {
		return Result{}, fmt.Errorf("chatledger: chat %s: session owner: %w", chat.ChatID, ownerErr)
	}
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", errUnconvertible, err)
	}
	err = store.Import(ctx, chat.ChatID, res.Rows)
	if !errors.Is(err, ledgerstore.ErrChatNotEmpty) {
		return res, err
	}
	held, readErr := store.Read(ctx, chat.ChatID, 0, 0)
	if readErr != nil {
		return Result{}, fmt.Errorf("chatledger: chat %s: %w", chat.ChatID, readErr)
	}
	if !reflect.DeepEqual(held, res.Rows) {
		return res, err
	}
	return res, nil
}

func verify(ctx context.Context, h home.Home, converted map[string]map[string]bool) error {
	if len(converted) == 0 {
		return nil
	}
	return withStore(h, func(store *ledgerstore.Store) error {
		for _, chatID := range slices.Sorted(maps.Keys(converted)) {
			st, err := store.State(ctx, chatID)
			if err != nil {
				return fmt.Errorf("chatledger: chat %s: %w", chatID, err)
			}
			if err := AtRest(st, converted[chatID]); err != nil {
				return fmt.Errorf("chatledger: chat %s: %w", chatID, err)
			}
		}
		return nil
	})
}

// withStore runs fn over the home's chat ledger, closed before it returns.
func withStore(h home.Home, fn func(*ledgerstore.Store) error) error {
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
	store, err := ledgerstore.Open(db, time.Now)
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
