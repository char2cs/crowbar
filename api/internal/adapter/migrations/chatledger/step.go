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

// errNotAsConverted is a converted chat whose ledger is not the rows imported.
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

// legacyHome is every legacy chat with its census count.
type legacyHome struct {
	chats  []LegacyChat
	counts map[string]int64
}

// convertAll imports, in order, each chat the boot owes a conversion. A chat
// whose ledger already has rows, or whose records cannot be converted, is
// skipped and noted with why; one whose import fails is skipped until the next
// boot. Only an unreadable session index, or every import failing, stops it.
func convertAll(
	ctx context.Context, log *slog.Logger, store ledgerStore, cp checkpoints, legacy legacyHome,
) (migrate.Counts, error) {
	unmarked := migrate.Counts{}
	owed := recorded{marked: cp.Marked, noted: cp.Noted}
	var failed error
	imported := 0
	for _, chat := range legacy.chats {
		if err := ctx.Err(); err != nil {
			return nonZero(unmarked), err
		}
		n := legacy.counts[chat.ChatID]
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
			log.WarnContext(ctx, "chatledger: skipped records", "chat_id", chat.ChatID, "skipped", clip(strings.Join(res.Skipped, "; "), noteBytes))
		}
		if errors.Is(err, errSessionIndex) || interrupted(ctx, err) {
			return nonZero(unmarked), err
		}
		if err != nil {
			final := errors.Is(err, ledgerstore.ErrChatNotEmpty) || errors.Is(err, errUnconvertible)
			if !final {
				failed = cmp.Or(failed, err)
			}
			if err := skip(ctx, log, cp, chat.ChatID, n, err, final); err != nil {
				return nonZero(unmarked), err
			}
			unmarked.Add(migrate.Counts{"skipped_chats": 1})
			continue
		}
		counts := migrate.Counts{"rows": int64(len(res.Rows)), "skipped_rows": int64(len(res.Skipped)), legacyRecords: n}
		if err := cp.Mark(chat.ChatID, counts); err != nil {
			return nonZero(unmarked), fmt.Errorf("chatledger: chat %s: %w", chat.ChatID, err)
		}
		imported++
	}
	// Every import failing is the ledger, not a chat: the step fails on it.
	if failed != nil && imported == 0 {
		return nonZero(unmarked), failed
	}
	return nonZero(unmarked), nil
}

func interrupted(ctx context.Context, err error) bool {
	return ctx.Err() != nil && errors.Is(err, ctx.Err())
}

// skip notes why chat was not converted and by which converter: no later boot
// tries it again until a newer converter might convert it. A skip that is not
// final, an import that failed, is noted by no converter: the next boot retries.
func skip(ctx context.Context, log *slog.Logger, cp checkpoints, chatID string, n int64, why error, final bool) error {
	msg, by := "chatledger: skipped a chat that cannot be converted", int64(converterVersion)
	switch {
	case errors.Is(why, ledgerstore.ErrChatNotEmpty):
		msg = "chatledger: skipped a chat whose ledger already holds rows"
	case !final:
		msg, by = "chatledger: skipped a chat whose import failed; the next boot tries it again", 0
	}
	detail := clip(why.Error(), noteBytes)
	log.WarnContext(ctx, msg, "chat_id", chatID, "err", detail)
	note := migrate.Note{Detail: detail, Counts: migrate.Counts{legacyRecords: n, "converter": by}}
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
	err = importSafely(ctx, log, store, chat.ChatID, res.Rows)
	if errors.Is(err, ledgerstore.ErrChatNotEmpty) {
		held, readErr := store.Read(ctx, chat.ChatID, 0, 0)
		if readErr != nil {
			return Result{}, fmt.Errorf("chatledger: chat %s: %w", chat.ChatID, readErr)
		}
		if !reflect.DeepEqual(held, res.Rows) {
			return res, err
		}
		err = nil
	}
	if err != nil {
		return res, err
	}
	return res, landedAsWritten(ctx, store, chat, int64(len(res.Rows)))
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
		return fmt.Errorf("chatledger: chat %s: %w", chat.ChatID, err)
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
