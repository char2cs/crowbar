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

var errNoCatchUp = errors.New("chatledger: the ledger does not end where the legacy records go on")

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

func conversational(rows []ledger.Row) []ledger.Row {
	out := make([]ledger.Row, 0, len(rows))
	for _, r := range rows {
		if r.Kind != ledger.KindRunnerStarted && r.Kind != ledger.KindRunnerExited {
			out = append(out, r)
		}
	}
	return out
}

func sameRows(a, b []ledger.Row) bool {
	for i := range a {
		x, y := a[i], b[i]
		if x.Kind != y.Kind || x.TurnID != y.TurnID || x.Scope != y.Scope || !x.At.Equal(y.At) || !bytes.Equal(x.Body, y.Body) {
			return false
		}
	}
	return true
}
