package chatledger

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"slices"
	"time"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// Impl converts each legacy chat into the ledger once, checkpointed per chat,
// and verifies every chat it converted is at rest.
func Impl() migrate.Impl {
	// Verify checks what Apply converted: the engine runs Apply first on every
	// attempt, and Apply also fills in the chats earlier attempts marked done.
	converted := map[string]landed{}
	return migrate.Impl{
		Apply: func(ctx context.Context, env migrate.Env, cp *migrate.Checkpoints) (migrate.Counts, error) {
			return apply(ctx, env, cp, converted)
		},
		Verify: func(ctx context.Context, env migrate.Env) error {
			return verify(ctx, env.Home, converted)
		},
	}
}

// landed is a chat this attempt, or an unverified earlier one, imported: the
// runners it is left live on, and how many rows the import wrote.
type landed struct {
	adopted map[string]bool
	rows    int64
}

// errNotAsConverted is a converted chat whose ledger is not the rows imported.
var errNotAsConverted = errors.New("chatledger: the ledger is not what the conversion wrote")

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
	Settled(key string) bool
	Marked(key string) (migrate.Counts, bool)
	Mark(key string, n migrate.Counts) error
	Noted(key string) (migrate.Note, bool)
	Note(key string, n migrate.Note) error
}

func apply(ctx context.Context, env migrate.Env, cp *migrate.Checkpoints, converted map[string]landed) (migrate.Counts, error) {
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
			total, err = convertAll(ctx, env.Log, store, cp, legacyHome{chats, counts}, converted)
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

// legacyHome is every legacy chat with its census count.
type legacyHome struct {
	chats  []LegacyChat
	counts map[string]int64
}

// convertAll imports, in order, each chat the boot owes a conversion. A chat
// whose ledger already has rows, or whose records cannot be converted, is
// skipped and noted with why; a store failure stops the step.
func convertAll(
	ctx context.Context, log *slog.Logger, store ledgerStore, cp checkpoints,
	legacy legacyHome, converted map[string]landed,
) (migrate.Counts, error) {
	unmarked := migrate.Counts{}
	owed := recorded{marked: cp.Marked, noted: cp.Noted}
	for _, chat := range legacy.chats {
		if err := ctx.Err(); err != nil {
			return nonZero(unmarked), err
		}
		n := legacy.counts[chat.ChatID]
		if marked, ok := cp.Marked(chat.ChatID); ok && !cp.Settled(chat.ChatID) {
			converted[chat.ChatID] = landed{adoptedBy(chat), marked["rows"]}
		}
		switch owed.owed(chat.ChatID, n) {
		case dutyNone:
			continue
		case dutyRecordChange:
			if err := recordChange(ctx, log, cp, chat.ChatID, n); err != nil {
				return nonZero(unmarked), err
			}
			continue
		case dutyConvert:
		}
		res, err := importChat(ctx, log, store, chat)
		if len(res.Skipped) > 0 {
			log.WarnContext(ctx, "chatledger: skipped records", "chat_id", chat.ChatID, "skipped", res.Skipped)
		}
		if errors.Is(err, ledgerstore.ErrChatNotEmpty) || errors.Is(err, errUnconvertible) {
			if err := skip(ctx, log, cp, chat.ChatID, n, err); err != nil {
				return nonZero(unmarked), err
			}
			unmarked.Add(migrate.Counts{"skipped_chats": 1})
			continue
		}
		if err != nil {
			return nonZero(unmarked), err
		}
		counts := migrate.Counts{"rows": int64(len(res.Rows)), "skipped_rows": int64(len(res.Skipped)), legacyRecords: n}
		if err := cp.Mark(chat.ChatID, counts); err != nil {
			return nonZero(unmarked), fmt.Errorf("chatledger: chat %s: %w", chat.ChatID, err)
		}
		converted[chat.ChatID] = landed{adoptedBy(chat), counts["rows"]}
	}
	return nonZero(unmarked), nil
}

// skip notes why chat was not converted and by which converter: no later boot
// tries it again until a newer converter might convert it.
func skip(ctx context.Context, log *slog.Logger, cp checkpoints, chatID string, n int64, why error) error {
	msg := "chatledger: skipped a chat that cannot be converted"
	if errors.Is(why, ledgerstore.ErrChatNotEmpty) {
		msg = "chatledger: skipped a chat whose ledger already holds rows"
	}
	log.WarnContext(ctx, msg, "chat_id", chatID, "err", why)
	note := migrate.Note{Detail: why.Error(), Counts: migrate.Counts{legacyRecords: n, "converter": converterVersion}}
	if err := cp.Note(skippedKey(chatID), note); err != nil {
		return fmt.Errorf("chatledger: chat %s: %w", chatID, err)
	}
	return nil
}

// recordChange notes a converted chat an older build has since added to. The
// ledger moved on from the conversion, so the additions are named, not merged.
func recordChange(ctx context.Context, log *slog.Logger, cp checkpoints, chatID string, n int64) error {
	counts, _ := cp.Marked(chatID)
	log.WarnContext(ctx, "chatledger: an older build added to a converted chat; the additions are not in the ledger",
		"chat_id", chatID, "legacy_records", n, "converted_records", counts[legacyRecords])
	note := migrate.Note{
		Detail: "legacy records added after the conversion are not in the ledger",
		Counts: migrate.Counts{legacyRecords: n, "converted_records": counts[legacyRecords]},
	}
	if err := cp.Note(changedKey(chatID), note); err != nil {
		return fmt.Errorf("chatledger: chat %s: %w", chatID, err)
	}
	return nil
}

func nonZero(c migrate.Counts) migrate.Counts {
	if len(c) == 0 {
		return nil
	}
	return c
}

// importChat converts chat and imports it. A chat already holding exactly
// those rows is an import an interrupted attempt committed and never marked.
func importChat(ctx context.Context, log *slog.Logger, store ledgerStore, chat LegacyChat) (Result, error) {
	var ownerErr error
	res, err := convertSafely(ctx, log, chat, func(provider, sessionID string) (string, bool) {
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

func verify(ctx context.Context, h home.Home, converted map[string]landed) error {
	if len(converted) == 0 {
		return nil
	}
	return withStore(ctx, h, func(store *ledgerstore.Store) error {
		for _, chatID := range slices.Sorted(maps.Keys(converted)) {
			st, err := store.State(ctx, chatID)
			if err != nil {
				return fmt.Errorf("chatledger: chat %s: %w", chatID, err)
			}
			if want := converted[chatID].rows; st.Head() != want {
				return fmt.Errorf("%w: chat %s: it holds %d rows, the import wrote %d", errNotAsConverted, chatID, st.Head(), want)
			}
			if err := AtRest(st, converted[chatID].adopted); err != nil {
				return fmt.Errorf("chatledger: chat %s: %w", chatID, err)
			}
		}
		return nil
	})
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
