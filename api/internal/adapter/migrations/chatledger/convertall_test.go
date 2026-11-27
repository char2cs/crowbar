package chatledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

type fakeLedger struct {
	ownerErr  error
	onOwner   func()
	onImport  func(chatID string) error
	imported  []string
	importing map[string][]ledger.Row
	landed    map[string][]ledger.Row
	folds     int
}

func (f *fakeLedger) SessionOwner(context.Context, string, string) (string, bool, error) {
	if f.onOwner != nil {
		f.onOwner()
	}
	return "", false, f.ownerErr
}

func (f *fakeLedger) Import(_ context.Context, chatID string, rows []ledger.Row) error {
	if f.onImport != nil {
		if err := f.onImport(chatID); err != nil {
			return err
		}
	}
	f.imported = append(f.imported, chatID)
	if f.importing == nil {
		f.importing = map[string][]ledger.Row{}
	}
	f.importing[chatID] = rows
	return nil
}

func (f *fakeLedger) State(ctx context.Context, chatID string) (ledger.State, error) {
	f.folds++
	rows, err := f.Read(ctx, chatID, 0, 0)
	return ledger.Fold(chatID, rows), err
}

func (f *fakeLedger) Read(_ context.Context, chatID string, _ int64, _ int) ([]ledger.Row, error) {
	return append(slices.Clone(f.importing[chatID]), f.landed[chatID]...), nil
}

func (f *fakeLedger) Append(context.Context, string, ...ledger.Row) ([]ledger.Row, ledger.State, error) {
	return nil, ledger.State{}, nil
}

func (f *fakeLedger) ImportAfter(context.Context, string, int64, []ledger.Row) error { return nil }

type marks map[string]migrate.Counts

func (m marks) Settled(string) bool { return false }

func (m marks) Marked(key string) (migrate.Counts, bool) { n, ok := m[key]; return n, ok }

func (m marks) Mark(key string, n migrate.Counts) error { m[key] = n; return nil }

func (m marks) Noted(string) (migrate.Note, bool) { return migrate.Note{}, false }

func (m marks) Note(string, migrate.Note) error { return nil }

func TestConvertAll_StopsBeforeTheNextChatOnceCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &fakeLedger{onImport: func(string) error { cancel(); return nil }}
	cp := marks{}

	_, err := convertAll(ctx, slog.Default(), store, cp, inMemory(t, chatB(), chatA()))

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"chat-b"}, store.imported, "no chat is imported after the cancel")
	assert.Contains(t, cp, "chat-b")
}

func TestConvertAll_AsksTheStoreForNoChatsState(t *testing.T) {
	store := &fakeLedger{}

	_, err := convertAll(context.Background(), slog.Default(), store, marks{}, inMemory(t, chatB(), chatA()))

	require.NoError(t, err)
	assert.Equal(t, []string{"chat-b", "chat-a"}, store.imported)
	assert.Zero(t, store.folds, "each import is checked against the rows read back")
}

func TestConvertAll_ASessionOwnerFailureStopsTheStep(t *testing.T) {
	broken := errors.New("session index unreadable")
	store := &fakeLedger{ownerErr: broken}
	cp := marks{}

	_, err := convertAll(context.Background(), slog.Default(), store, cp, inMemory(t, chatB()))

	require.ErrorIs(t, err, broken)
	require.NotErrorIs(t, err, errUnconvertible)
	assert.Empty(t, store.imported)
	assert.Empty(t, cp)
}

func TestConvertAll_AConversionThatPanicsSkipsOnlyItsChat(t *testing.T) {
	calls := 0
	store := &fakeLedger{onOwner: func() {
		if calls++; calls == 1 {
			panic("converter bug")
		}
	}}
	cp := marks{}
	log, got := capture(func(r slog.Record) bool { return r.Level == slog.LevelError })

	n, err := convertAll(context.Background(), log, store, cp, inMemory(t, chatB(), chatA()))

	require.NoError(t, err)
	assert.Equal(t, migrate.Counts{"skipped_chats": 1}, n)
	assert.Equal(t, []string{"chat-a"}, store.imported)
	require.Len(t, *got, 1)
	assert.Contains(t, testutil.Attrs((*got)[0])["stack"].String(), "convertSafely")
}

func TestConvertAll_AChatWhoseImportFailsSkipsOnlyThatChat(t *testing.T) {
	for name, fail := range map[string]func(){
		"fails":  func() {},
		"panics": func() { panic("gorm callback bug") },
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeLedger{onImport: func(chatID string) error {
				if chatID != "chat-b" {
					return nil
				}
				fail()
				return errors.New("constraint failed")
			}}
			cp := notes{marks: marks{}, notes: map[string]migrate.Note{}}

			n, err := convertAll(context.Background(), slog.Default(), store, cp, inMemory(t, chatB(), chatA()))

			require.NoError(t, err)
			retry := name == "fails"
			want := migrate.Counts{"skipped_chats": 1}
			if retry {
				want[retryingChats] = 1
			}
			assert.Equal(t, want, n)
			assert.Equal(t, []string{"chat-a"}, store.imported)
			assert.Equal(t, retry, cp.notes[skippedKey("chat-b")].Counts["converter"] == 0, "a failed import is retried, a panic is not")
		})
	}
}

func TestConvertAll_EveryImportFailingFailsTheStep(t *testing.T) {
	full := errors.New("database or disk is full")
	store := &fakeLedger{onImport: func(string) error { return full }}
	cp := notes{marks: marks{}, notes: map[string]migrate.Note{}}

	_, err := convertAll(context.Background(), slog.Default(), store, cp, inMemory(t, chatB(), chatA()))

	require.ErrorIs(t, err, full)
	assert.Empty(t, cp.notes)
}

func TestConvertAll_AnImportThatDidNotLandAsConvertedIsSkippedForGood(t *testing.T) {
	stray, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerStopped})
	require.NoError(t, err)
	stray.Seq = int64(len(convert(t, chatB(), nobody).Rows) + 1)
	store := &fakeLedger{landed: map[string][]ledger.Row{"chat-b": {stray}}}
	cp := notes{marks: marks{}, notes: map[string]migrate.Note{}}
	log, warned := capture(func(r slog.Record) bool { return strings.HasPrefix(r.Message, "chatledger: skipped a chat") })

	n, err := convertAll(context.Background(), log, store, cp, inMemory(t, chatB(), chatA()))

	require.NoError(t, err)
	assert.Equal(t, migrate.Counts{"skipped_chats": 1}, n)
	assert.Equal(t, int64(converterVersion), cp.notes[skippedKey("chat-b")].Counts["converter"], "not tried again")
	require.Len(t, *warned, 1)
	assert.Equal(t, "chatledger: skipped a chat whose import did not land as converted; its rows are kept, it is not tried again", (*warned)[0].Message)
}

type storeFault int

func (f storeFault) Error() string { return fmt.Sprintf("database or disk is full (%d)", int(f)) }

func (f storeFault) Code() int { return int(f) }

func TestConvertAll_AStoreFaultStopsTheStepAndCostsNoAttempt(t *testing.T) {
	full := true
	store := &fakeLedger{onImport: func(chatID string) error {
		if chatID == "chat-a" && full {
			return fmt.Errorf("ledger: append to %s: %w", chatID, storeFault(13))
		}
		return nil
	}}
	cp := notes{marks: marks{}, notes: map[string]migrate.Note{}}
	legacy := inMemory(t, chatB(), chatA())

	_, err := convertAll(context.Background(), slog.Default(), store, cp, legacy)

	require.ErrorIs(t, err, storeFault(13))
	assert.Contains(t, cp.marks, "chat-b")
	assert.Empty(t, cp.notes, "no chat is charged for the store")
	full = false
	_, err = convertAll(context.Background(), slog.Default(), store, cp, legacy)
	require.NoError(t, err)
	assert.Contains(t, cp.marks, "chat-a")
}

func inMemory(t *testing.T, chats ...LegacyChat) legacyHome {
	t.Helper()
	byID := map[string]LegacyChat{}
	var ids []string
	for _, c := range chats {
		byID[c.ChatID] = c
		ids = append(ids, c.ChatID)
	}
	return legacyHome{ids: ids, load: func(_ context.Context, id string) (LegacyChat, error) {
		return byID[id], nil
	}}
}

func TestConvertAll_ReadsOnlyTheChatsItConverts(t *testing.T) {
	store := &fakeLedger{}
	cp := notes{marks: marks{"chat-a": {legacyRecords: 1}}, notes: map[string]migrate.Note{}}
	legacy := inMemory(t, chatB(), chatA())
	load := legacy.load
	var read []string
	legacy.load = func(ctx context.Context, id string) (LegacyChat, error) {
		read = append(read, id)
		return load(ctx, id)
	}
	legacy.counts = map[string]int64{"chat-a": 1}

	_, err := convertAll(context.Background(), slog.Default(), store, cp, legacy)

	require.NoError(t, err)
	assert.Equal(t, []string{"chat-b"}, read)
}
