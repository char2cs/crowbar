package chatledger

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	converted := map[string]map[string]bool{} // chat → the runners it is left live on
	return migrate.Impl{
		Apply: func(ctx context.Context, env migrate.Env, cp *migrate.Checkpoints) (migrate.Counts, error) {
			return apply(ctx, env.Home, cp, converted)
		},
		Verify: func(ctx context.Context, env migrate.Env) error {
			return verify(ctx, env.Home, converted)
		},
	}
}

func apply(ctx context.Context, h home.Home, cp *migrate.Checkpoints, converted map[string]map[string]bool) (migrate.Counts, error) {
	chats, err := ReadLegacy(ctx, h)
	if err != nil || len(chats) == 0 {
		return nil, err
	}
	slices.SortStableFunc(chats, func(a, b LegacyChat) int {
		return cmp.Or(oldest(a).Compare(oldest(b)), cmp.Compare(a.ChatID, b.ChatID))
	})
	var skippedChats int64
	err = withStore(h, func(store *ledgerstore.Store) error {
		for _, chat := range chats {
			if err := ctx.Err(); err != nil {
				return err
			}
			if cp.Done(chat.ChatID) {
				converted[chat.ChatID] = adoptedBy(chat)
				continue
			}
			n, skipped, err := importChat(ctx, store, chat)
			if errors.Is(err, ledgerstore.ErrChatNotEmpty) {
				skippedChats++
				continue
			}
			if err != nil {
				return err
			}
			if err := cp.Mark(chat.ChatID, migrate.Counts{"rows": n, "skipped_rows": skipped}); err != nil {
				return err
			}
			converted[chat.ChatID] = adoptedBy(chat)
		}
		return nil
	})
	if skippedChats == 0 {
		return nil, err
	}
	return migrate.Counts{"skipped_chats": skippedChats}, err
}

// importChat converts chat and imports it. A chat already holding exactly
// those rows is an import an interrupted attempt committed and never marked.
func importChat(ctx context.Context, store *ledgerstore.Store, chat LegacyChat) (rows, skipped int64, err error) {
	var ownerErr error
	res, err := ConvertChat(chat, func(provider, sessionID string) (string, bool) {
		chatID, ok, err := store.SessionOwner(ctx, provider, sessionID)
		ownerErr = errors.Join(ownerErr, err)
		return chatID, ok
	})
	if err = errors.Join(err, ownerErr); err != nil {
		return 0, 0, err
	}
	rows, skipped = int64(len(res.Rows)), int64(len(res.Skipped))
	err = store.Import(ctx, chat.ChatID, res.Rows)
	if !errors.Is(err, ledgerstore.ErrChatNotEmpty) {
		return rows, skipped, err
	}
	held, readErr := store.Read(ctx, chat.ChatID, 0, 0)
	if readErr != nil {
		return 0, 0, readErr
	}
	if !reflect.DeepEqual(held, res.Rows) {
		return 0, 0, err
	}
	return rows, skipped, nil
}

func verify(ctx context.Context, h home.Home, converted map[string]map[string]bool) error {
	if len(converted) == 0 {
		return nil
	}
	return withStore(h, func(store *ledgerstore.Store) error {
		for _, chatID := range slices.Sorted(maps.Keys(converted)) {
			st, err := store.State(ctx, chatID)
			if err != nil {
				return err
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
