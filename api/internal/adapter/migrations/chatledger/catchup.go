package chatledger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// errNoCatchUp is a converted chat whose ledger does not end where the legacy
// files' newer records begin: written to by this build since, or changed in place.
var errNoCatchUp = errors.New("chatledger: the ledger does not end where the legacy records go on")

// catchUp imports into a converted chat what an older build recorded in the
// legacy files since the conversion, once per legacy count. A chat it cannot
// extend without merging two histories is only noted, its legacy files kept.
func catchUp(ctx context.Context, log *slog.Logger, store ledgerStore, cp checkpoints, legacy legacyHome, chatID string, n int64) error {
	chat, err := legacy.load(ctx, chatID)
	if err != nil {
		return err
	}
	added, err := extend(ctx, log, store, chat)
	if errors.Is(err, errNoCatchUp) || errors.Is(err, errUnconvertible) {
		return recordChange(ctx, log, cp, chatID, n)
	}
	if err != nil {
		return err
	}
	log.InfoContext(ctx, "chatledger: imported what an older build added to a converted chat", "chat_id", chatID, "rows", added)
	note := migrate.Note{
		Detail: "legacy records added after the conversion were imported",
		Counts: migrate.Counts{legacyRecords: n, "imported_rows": added},
	}
	if err := cp.Note(changedKey(chatID), note); err != nil {
		return fmt.Errorf("chatledger: chat %s: %w", chatID, err)
	}
	return nil
}

// extend appends to chat's ledger the rows converting all its legacy records
// now gives past those the ledger already ends with, keeping their times, and
// reports how many it appended.
func extend(ctx context.Context, log *slog.Logger, store ledgerStore, chat LegacyChat) (int64, error) {
	res, err := convertOver(ctx, log, store, chat)
	if err != nil {
		return 0, err
	}
	held, err := store.Read(ctx, chat.ChatID, 0, 0)
	if err != nil {
		return 0, fmt.Errorf("chatledger: chat %s: %w", chat.ChatID, err)
	}
	tail, ok := beyond(conversational(held), conversational(res.Rows))
	if !ok {
		return 0, fmt.Errorf("%w: chat %s", errNoCatchUp, chat.ChatID)
	}
	if len(tail) == 0 {
		return 0, nil
	}
	st, err := store.State(ctx, chat.ChatID)
	if err != nil {
		return 0, fmt.Errorf("chatledger: chat %s: %w", chat.ChatID, err)
	}
	rows, err := after(st, tail)
	if err != nil {
		return 0, fmt.Errorf("%w: chat %s: %w", errNoCatchUp, chat.ChatID, err)
	}
	if err := store.ImportAfter(ctx, chat.ChatID, st.Head(), rows); err != nil {
		if stops(ctx, err) {
			return 0, err
		}
		return 0, fmt.Errorf("%w: chat %s: %w", errNoCatchUp, chat.ChatID, err)
	}
	return int64(len(rows)), nil
}

// after numbers tail on from st and checks the chat is left as the conversion
// leaves one: each row allowed where it lands, and the whole at rest.
func after(st ledger.State, tail []ledger.Row) ([]ledger.Row, error) {
	next := st.Clone()
	live := map[string]bool{}
	if r, ok := st.LiveRunner(); ok {
		live[r.RunnerID] = true
	}
	rows := make([]ledger.Row, 0, len(tail))
	for _, r := range tail {
		r.ChatID, r.Seq = st.ChatID, next.Head()+1
		if err := ledger.Check(next, r); err != nil {
			return nil, err
		}
		next.Apply(r)
		rows = append(rows, r)
	}
	return rows, AtRest(next, live)
}

// beyond is what conv holds past the longest run of rows held ends with and
// conv begins with. With no such run, conv is whole only when it comes wholly
// after held, as legacy files written afresh after their retirement do.
func beyond(held, conv []ledger.Row) ([]ledger.Row, bool) {
	for j := min(len(held), len(conv)); j > 0; j-- {
		if sameRows(held[len(held)-j:], conv[:j]) {
			return conv[j:], true
		}
	}
	if len(conv) == 0 || len(held) == 0 {
		return conv, true
	}
	if !conv[0].At.After(held[len(held)-1].At) {
		return nil, false
	}
	turns := map[string]bool{}
	for _, r := range held {
		turns[r.TurnID] = r.TurnID != ""
	}
	for _, r := range conv {
		if turns[r.TurnID] {
			return nil, false
		}
	}
	return conv, true
}

// conversational is rows without the runner rows a boot's reconcile adds and
// a legacy runner table forgets: they say nothing of the conversation.
func conversational(rows []ledger.Row) []ledger.Row {
	out := make([]ledger.Row, 0, len(rows))
	for _, r := range rows {
		if r.Kind != ledger.KindRunnerStarted && r.Kind != ledger.KindRunnerExited {
			out = append(out, r)
		}
	}
	return out
}

// sameRows reports whether a and b say the same, row for row, wherever they
// are numbered.
func sameRows(a, b []ledger.Row) bool {
	for i := range a {
		x, y := a[i], b[i]
		if x.Kind != y.Kind || x.TurnID != y.TurnID || x.Scope != y.Scope || !x.At.Equal(y.At) || !bytes.Equal(x.Body, y.Body) {
			return false
		}
	}
	return true
}
