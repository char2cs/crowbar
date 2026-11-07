package chatledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// legacyHome is every legacy chat, in the order to convert them, with its
// census count; load reads one chat's records only when it is converted.
type legacyHome struct {
	ids    []string
	load   func(ctx context.Context, chatID string) (LegacyChat, error)
	counts map[string]int64
}

// convertAll imports, in order, each chat the boot owes a conversion. A chat with
// ledger rows or unconvertible records is skipped with why; a failed import waits for
// the next boot. A whole-store fault or every import failing stops it, charging no chat.
func convertAll(
	ctx context.Context, log *slog.Logger, store ledgerStore, cp checkpoints, legacy legacyHome,
) (migrate.Counts, error) {
	unmarked := migrate.Counts{}
	owed := recorded{marked: cp.Marked, noted: cp.Noted}
	var failed []failedImport
	imported := 0
	for _, chatID := range legacy.ids {
		if err := ctx.Err(); err != nil {
			return nonZero(unmarked), err
		}
		n := legacy.counts[chatID]
		convert, err := due(ctx, log, cp, owed.owed(chatID, n), chatID, n)
		if err != nil || !convert {
			if err != nil {
				return nonZero(unmarked), err
			}
			continue
		}
		chat, err := legacy.load(ctx, chatID)
		if err != nil {
			return nonZero(unmarked), err
		}
		landed, err := attempt(ctx, log, store, cp, chat, n)
		switch {
		case err != nil && !stops(ctx, err) && retried(err) != nil:
			failed = append(failed, failedImport{chatID: chat.ChatID, n: n, err: err})
		case err != nil:
			return nonZero(unmarked), err
		case landed:
			imported++
		default:
			unmarked.Add(migrate.Counts{"skipped_chats": 1})
		}
	}
	waiting, err := charge(ctx, log, cp, failed, imported)
	if err == nil && len(failed) > 0 {
		unmarked.Add(migrate.Counts{"skipped_chats": int64(len(failed))})
	}
	if waiting > 0 {
		unmarked.Add(migrate.Counts{retryingChats: waiting})
	}
	return nonZero(unmarked), err
}

// attempt imports chat and marks it, or gives it up, and reports whether it
// landed. An import that failed is its error, for the caller to retry or stop.
func attempt(ctx context.Context, log *slog.Logger, store ledgerStore, cp checkpoints, chat LegacyChat, n int64) (bool, error) {
	res, err := importChat(ctx, log, store, chat)
	if err != nil && (stops(ctx, err) || retried(err) != nil) {
		return false, err
	}
	if err != nil {
		return false, giveUp(ctx, log, store, cp, chat.ChatID, n, err)
	}
	counts := migrate.Counts{"rows": int64(len(res.Rows)), "skipped_rows": int64(len(res.Skipped)), legacyRecords: n}
	if err := cp.Mark(chat.ChatID, counts); err != nil {
		return false, fmt.Errorf("chatledger: chat %s: %w", chat.ChatID, err)
	}
	return true, nil
}

// stops reports whether err stops the step: an unreadable session index, a
// cancelled boot, or a fault of the whole store.
func stops(ctx context.Context, err error) bool {
	return errors.Is(err, errSessionIndex) || interrupted(ctx, err) || storeWide(err)
}

// due does what duty owes chatID short of converting it, and reports whether
// it owes the conversion.
func due(ctx context.Context, log *slog.Logger, cp checkpoints, duty duty, chatID string, n int64) (bool, error) {
	switch duty {
	case dutyConvert:
		return true, nil
	case dutyRecordChange:
		return false, recordChange(ctx, log, cp, chatID, n)
	case dutyNone:
	}
	return false, nil
}

// giveUp skips chat for good. One holding rows of its own is told, in a row
// after them, that its legacy history was not imported, before the note.
func giveUp(ctx context.Context, log *slog.Logger, store ledgerStore, cp checkpoints, chatID string, n int64, why error) error {
	if errors.Is(why, ledgerstore.ErrChatNotEmpty) {
		if err := sayNotImported(ctx, store, chatID); err != nil {
			return err
		}
	}
	return skip(ctx, log, cp, chatID, n, why)
}

// sayNotImported appends the historyNotImported marker to chatID, unless an
// earlier attempt already did: an attempt killed before its note says it once.
func sayNotImported(ctx context.Context, store ledgerStore, chatID string) error {
	held, err := store.Read(ctx, chatID, 0, 0)
	if err != nil {
		return fmt.Errorf("chatledger: chat %s: %w", chatID, err)
	}
	for _, r := range held {
		if b, err := ledger.DecodeBody[ledger.Marker](r); r.Kind == ledger.KindMarker && err == nil && b.Detail == historyNotImported {
			return nil
		}
	}
	r, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerUnreadable, Detail: historyNotImported})
	if err == nil {
		_, _, err = store.Append(ctx, chatID, r)
	}
	if err != nil {
		return fmt.Errorf("chatledger: chat %s: %w", chatID, err)
	}
	return nil
}

// historyNotImported is what a chat that holds rows of its own says when its
// legacy history cannot be imported under them.
const historyNotImported = "This chat's history from before the upgrade could not be imported; the old daemon's files still hold it"

// failedImport is a chat whose import failed in this run, for the next boot.
type failedImport struct {
	chatID string
	n      int64
	err    error
}

// charge notes each failed import as one attempt of its chat, and counts the
// chats left owed a retry. Two failing and none landing is the ledger, not a
// chat: the step fails, charging none.
func charge(ctx context.Context, log *slog.Logger, cp checkpoints, failed []failedImport, imported int) (int64, error) {
	if len(failed) > 1 && imported == 0 {
		return 0, failed[0].err
	}
	var owed int64
	for _, f := range failed {
		if err := skip(ctx, log, cp, f.chatID, f.n, f.err); err != nil {
			return 0, err
		}
		if note, _ := cp.Noted(skippedKey(f.chatID)); note.Counts["converter"] == 0 {
			owed++
		}
	}
	return owed, nil
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

// skip notes why chat was not converted and by which converter, so no later boot
// retries it until a newer converter. A failed import is noted by no converter and
// retried on later boots, importAttempts times in all, before it is given up.
func skip(ctx context.Context, log *slog.Logger, cp checkpoints, chatID string, n int64, why error) error {
	msg, counts := "chatledger: skipped a chat that cannot be converted", migrate.Counts{legacyRecords: n, "converter": converterVersion}
	level := slog.LevelWarn
	switch {
	case errors.Is(why, ledgerstore.ErrChatNotEmpty):
		msg = "chatledger: skipped a chat whose ledger already holds rows; its legacy history is not imported, and the chat says so"
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
