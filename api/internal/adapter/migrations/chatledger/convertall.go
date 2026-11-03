package chatledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

// legacyHome is every legacy chat with its census count.
type legacyHome struct {
	chats  []LegacyChat
	counts map[string]int64
}

// convertAll imports, in order, each chat the boot owes a conversion. A chat
// whose ledger already has rows, or whose records cannot be converted, is
// skipped and noted with why; one whose import fails is skipped until the next
// boot. An unreadable session index, a fault of the whole store, or every
// import failing stops it, and charges no chat an attempt.
func convertAll(
	ctx context.Context, log *slog.Logger, store ledgerStore, cp checkpoints, legacy legacyHome,
) (migrate.Counts, error) {
	unmarked := migrate.Counts{}
	owed := recorded{marked: cp.Marked, noted: cp.Noted}
	var failed []failedImport
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
		if errors.Is(err, errSessionIndex) || interrupted(ctx, err) || storeWide(err) {
			return nonZero(unmarked), err
		}
		if retried(err) != nil {
			failed = append(failed, failedImport{chatID: chat.ChatID, n: n, err: err})
			continue
		}
		if err != nil {
			if err := skip(ctx, log, cp, chat.ChatID, n, err); err != nil {
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
	err := charge(ctx, log, cp, failed, imported)
	if err == nil && len(failed) > 0 {
		unmarked.Add(migrate.Counts{"skipped_chats": int64(len(failed))})
	}
	return nonZero(unmarked), err
}

// failedImport is a chat whose import failed in this run, for the next boot.
type failedImport struct {
	chatID string
	n      int64
	err    error
}

// charge notes each failed import as one attempt of its chat. Two failing and
// none landing is the ledger, not a chat: the step fails, charging none.
func charge(ctx context.Context, log *slog.Logger, cp checkpoints, failed []failedImport, imported int) error {
	if len(failed) > 1 && imported == 0 {
		return failed[0].err
	}
	for _, f := range failed {
		if err := skip(ctx, log, cp, f.chatID, f.n, f.err); err != nil {
			return err
		}
	}
	return nil
}

// The SQLite result codes of a fault in the store itself, not in one chat.
const (
	sqliteBusy     = 5
	sqliteLocked   = 6
	sqliteNoMem    = 7
	sqliteReadOnly = 8
	sqliteIOErr    = 10
	sqliteCorrupt  = 11
	sqliteFull     = 13
	sqliteCantOpen = 14
)

// storeWide reports whether err is the ledger database failing as a whole,
// read from its result code: no chat is to blame for it.
func storeWide(err error) bool {
	var coded interface{ Code() int }
	if !errors.As(err, &coded) {
		return false
	}
	switch coded.Code() & 0xff {
	case sqliteBusy, sqliteLocked, sqliteNoMem, sqliteReadOnly, sqliteIOErr, sqliteCorrupt, sqliteFull, sqliteCantOpen:
		return true
	}
	return false
}

// retried is err when it is an import failure the next boot tries again: not
// a chat whose ledger holds rows, one that cannot be converted, nor one that
// landed other than converted.
func retried(err error) error {
	if errors.Is(err, ledgerstore.ErrChatNotEmpty) || errors.Is(err, errUnconvertible) || errors.Is(err, errNotAsConverted) {
		return nil
	}
	return err
}

func interrupted(ctx context.Context, err error) bool {
	return ctx.Err() != nil && errors.Is(err, ctx.Err())
}

// skip notes why chat was not converted and by which converter: no later boot
// tries it again until a newer converter might convert it. An import that
// failed is noted by no converter, so the next boots retry it, importAttempts
// times in all: a chat failing at every boot is then given up like the others.
func skip(ctx context.Context, log *slog.Logger, cp checkpoints, chatID string, n int64, why error) error {
	msg, counts := "chatledger: skipped a chat that cannot be converted", migrate.Counts{legacyRecords: n, "converter": converterVersion}
	level := slog.LevelWarn
	switch {
	case errors.Is(why, ledgerstore.ErrChatNotEmpty):
		msg = "chatledger: skipped a chat whose ledger already holds rows"
	case errors.Is(why, errNotAsConverted):
		msg = "chatledger: skipped a chat whose import did not land as converted; its rows are kept, it is not tried again"
	case retried(why) != nil:
		counts["attempts"] = attempts(cp, chatID) + 1
		msg = "chatledger: skipped a chat whose import failed; the next boot tries it again"
		if counts["attempts"] < importAttempts {
			counts["converter"] = 0
			break
		}
		msg, level = fmt.Sprintf("chatledger: skipped a chat whose import failed at %d boots; it is not tried again until a newer converter", importAttempts), slog.LevelError
	}
	detail := clip(why.Error(), noteBytes)
	log.Log(ctx, level, msg, "chat_id", chatID, "err", detail, "attempts", counts["attempts"])
	if err := cp.Note(skippedKey(chatID), migrate.Note{Detail: detail, Counts: counts}); err != nil {
		return fmt.Errorf("chatledger: chat %s: %w", chatID, err)
	}
	return nil
}

// attempts is how many boots of this converter's line tried chatID's import
// and failed: none once a skip names a converter, which starts it over.
func attempts(cp checkpoints, chatID string) int64 {
	note, ok := cp.Noted(skippedKey(chatID))
	if !ok || note.Counts["converter"] != 0 {
		return 0
	}
	return note.Counts["attempts"]
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
