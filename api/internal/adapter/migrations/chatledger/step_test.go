package chatledger

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

const stepNumber = 6

// boot runs this step alone through the engine, as a daemon's pre-open half.
func boot(t *testing.T, h home.Home) {
	t.Helper()
	require.NoError(t, bootLogging(t, h, slog.Default()))
}

// bootLogging is boot with the session logging to log; it returns how the
// pre-open half ended.
func bootLogging(t *testing.T, h home.Home, log *slog.Logger) error {
	t.Helper()
	cat, err := migrate.NewCatalog(migrate.Step{Number: stepNumber, Name: "chat-ledger", Phase: migrate.PreOpen, Plan: Plan, Recurs: true})
	require.NoError(t, err)
	sess, err := migrate.Begin(context.Background(), h, cat, migrate.Options{Version: "v-test", Log: log})
	require.NoError(t, err)
	err = sess.RunPreOpen(context.Background(), migrate.Impls{stepNumber: Impl()})
	require.NoError(t, sess.Close())
	return err
}

// recorder keeps every record match accepts.
type recorder struct {
	match func(slog.Record) bool
	got   *[]slog.Record
}

func (r recorder) Enabled(context.Context, slog.Level) bool { return true }

func (r recorder) Handle(_ context.Context, rec slog.Record) error {
	if r.match(rec) {
		*r.got = append(*r.got, rec)
	}
	return nil
}

func (r recorder) WithAttrs([]slog.Attr) slog.Handler { return r }

func (r recorder) WithGroup(string) slog.Handler { return r }

func capture(match func(slog.Record) bool) (*slog.Logger, *[]slog.Record) {
	var got []slog.Record
	return slog.New(recorder{match: match, got: &got}), &got
}

// held is chatID's rows in h's ledger.
func held(t *testing.T, h home.Home, chatID string) []ledger.Row {
	t.Helper()
	var rows []ledger.Row
	require.NoError(t, withStore(t.Context(), h, func(s *ledgerstore.Store) error {
		var err error
		rows, err = s.Read(context.Background(), chatID, 0, 0)
		return err
	}))
	return rows
}

func appendTo(t *testing.T, h home.Home, chatID string, rows ...ledger.Row) {
	t.Helper()
	require.NoError(t, withStore(t.Context(), h, func(s *ledgerstore.Store) error {
		_, _, err := s.Append(context.Background(), chatID, rows...)
		return err
	}))
}

func TestApply_ConvertsEachChatOnceAndResumesFromItsCheckpoints(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA(), chatB())
	earlier := convert(t, chatB(), nobody).Rows
	appendTo(t, h, "chat-b", earlier...)
	testutil.WriteFile(t, h, "migrate/ledger.jsonl",
		`{"v":1,"kind":"run.start","run":"r1","pending":[6]}`+"\n"+
			`{"v":1,"kind":"checkpoint","run":"r1","step":6,"name":"chat-ledger","key":"chat-b","counts":{"rows":`+
			strconv.Itoa(len(earlier))+`}}`+"\n")

	boot(t, h)

	want := convert(t, chatA(), nobody)
	assert.Equal(t, want.Rows, held(t, h, "chat-a"))
	assert.Len(t, held(t, h, "chat-b"), len(earlier), "a chat an earlier attempt marked done is not converted again")
	applied := migratetest.Applied(t, h)[stepNumber]
	require.Len(t, applied, 1)
	assert.Equal(t, migrate.Counts{
		"rows": int64(len(want.Rows) + len(earlier)), "skipped_rows": int64(len(want.Skipped)), "legacy_records": records(chatA()),
	}, applied[0])

	before := testutil.TreeSnapshot(t, h)
	boot(t, h)
	assert.Equal(t, before, testutil.TreeSnapshot(t, h), "a second boot converts nothing")
}

// An attempt stopped after an import committed and before its checkpoint left
// exactly the rows a rerun produces: that chat is converted, not skipped.
func TestApply_AnImportAStoppedAttemptCommittedIsConverted(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA())
	want := convert(t, chatA(), nobody)
	require.NoError(t, withStore(t.Context(), h, func(s *ledgerstore.Store) error {
		return s.Import(context.Background(), "chat-a", want.Rows)
	}))

	boot(t, h)

	assert.Equal(t, want.Rows, held(t, h, "chat-a"))
	assert.Equal(t, []migrate.Counts{{
		"rows": int64(len(want.Rows)), "skipped_rows": int64(len(want.Skipped)), "legacy_records": records(chatA()),
	}},
		migratetest.Applied(t, h)[stepNumber])
}

func TestApply_AChatAlreadyInTheLedgerIsSkippedAndCounted(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA(), chatB())
	marker, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerStopped})
	require.NoError(t, err)
	appendTo(t, h, "chat-a", marker)

	boot(t, h)

	assert.Len(t, held(t, h, "chat-a"), 1, "a chat an intermediate build wrote is left as it is")
	assert.Equal(t, convert(t, chatB(), nobody).Rows, held(t, h, "chat-b"))
	applied := migratetest.Applied(t, h)[stepNumber]
	require.Len(t, applied, 1)
	assert.Equal(t, int64(1), applied[0]["skipped_chats"])
}

// The earlier chat claims a session two legacy chats name; the later one gets
// the conflict marker, and a rerun makes the same choice.
func TestApply_TheEarlierChatKeepsASessionTwoChatsName(t *testing.T) {
	h := testutil.FixtureHome(t)
	later := chatB()
	later.Conversations[0].ProviderID, later.Conversations[0].SessionID = "claude", "S-old"
	later.Conversations[0].FirstSeenAt = at(11, 0, 0)
	for i := range later.Turns {
		later.Turns[i].StartedAt = later.Turns[i].StartedAt.Add(3 * 60 * 60 * 1e9)
	}
	writeLegacy(t, h, chatA(), later)

	boot(t, h)

	var conflict bool
	for _, r := range held(t, h, "chat-b") {
		conflict = conflict || r.Kind == ledger.KindMarker && body[ledger.Marker](t, r).Kind == ledger.MarkerSessionConflict
		assert.NotEqual(t, ledger.KindSessionBound, r.Kind)
	}
	assert.True(t, conflict)
}

func TestApply_AHomeWithoutLegacyRecordsCreatesNoLedger(t *testing.T) {
	h := testutil.FixtureHome(t)

	boot(t, h)

	assert.NoFileExists(t, filepath.Join(h.Store(), ledgerstore.FileName))
	assert.Equal(t, []migrate.Counts{nil}, migratetest.Applied(t, h)[stepNumber])
}

func TestVerify_FailsOnAChatLeftWithOpenWork(t *testing.T) {
	h := testutil.FixtureHome(t)
	open, err := ledger.NewRow(ledger.KindUserMessage, "T-1", "", ledger.UserMessage{RequestID: "r1", Origin: ledger.OriginCrowbar})
	require.NoError(t, err)
	appendTo(t, h, "chat-x", open)
	live, err := ledger.NewRow(ledger.KindRunnerStarted, "", "", ledger.RunnerStarted{RunnerID: "run-1", Provider: "p"})
	require.NoError(t, err)
	appendTo(t, h, "chat-y", live)

	require.ErrorIs(t, verify(context.Background(), h, map[string]landed{"chat-x": {rows: 1}}), ErrNotAtRest)
	require.ErrorIs(t, verify(context.Background(), h, map[string]landed{"chat-y": {rows: 1}}), ErrNotAtRest)
	require.NoError(t, verify(context.Background(), h, map[string]landed{"chat-y": {adopted: map[string]bool{"run-1": true}, rows: 1}}),
		"a runner the old daemon left is the boot reconcile's to end")
}

// A chat whose ledger does not end at the rows its checkpoint says the
// import wrote is not what the conversion made.
func TestVerify_FailsOnAChatWhoseLedgerIsNotTheRowsItConverted(t *testing.T) {
	h := testutil.FixtureHome(t)
	marker, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerStopped})
	require.NoError(t, err)
	appendTo(t, h, "chat-x", marker, marker)

	require.ErrorIs(t, verify(context.Background(), h, map[string]landed{"chat-x": {rows: 1}}), errNotAsConverted)
	require.ErrorIs(t, verify(context.Background(), h, map[string]landed{"chat-x": {rows: 3}}), errNotAsConverted)
	require.NoError(t, verify(context.Background(), h, map[string]landed{"chat-x": {rows: 2}}))
}

// Each chat skipped, and each record a converted chat left out, is logged
// with the chat and why, so an upgrade's skips can be read back.
func TestApply_LogsWhatItSkippedAndWhy(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA(), chatB())
	marker, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerStopped})
	require.NoError(t, err)
	appendTo(t, h, "chat-b", marker)
	log, recs := capture(testutil.AtLeast(slog.LevelWarn))

	require.NoError(t, bootLogging(t, h, log))

	got := *recs
	require.Len(t, got, 2)
	assert.Equal(t, "chatledger: skipped a chat whose ledger already holds rows", got[0].Message)
	assert.Equal(t, "chat-b", testutil.Attrs(got[0])["chat_id"].String())
	assert.Equal(t, "chatledger: skipped records", got[1].Message)
	assert.Equal(t, "chat-a", testutil.Attrs(got[1])["chat_id"].String())
	assert.Equal(t, convert(t, chatA(), nobody).Skipped, testutil.Attrs(got[1])["skipped"].Any())
}

// A chat whose records do not restate is skipped and named; the boot goes on
// and its legacy files stay as they were, for a later build to convert.
func TestApply_AChatThatCannotBeConvertedIsSkippedNotAHalt(t *testing.T) {
	h := testutil.FixtureHome(t)
	broken := chatB()
	broken.Runners = []LegacyRunner{{ID: "", ChatID: broken.ChatID, ProviderID: "codex"}}
	writeLegacy(t, h, chatA(), broken)
	before := files(t, h.Store())
	log, got := capture(testutil.Message("chatledger: skipped a chat that cannot be converted"))

	require.NoError(t, bootLogging(t, h, log))

	assert.Empty(t, held(t, h, "chat-b"))
	assert.Equal(t, convert(t, chatA(), nobody).Rows, held(t, h, "chat-a"), "the other chats still convert")
	require.Len(t, *got, 1)
	assert.Equal(t, "chat-b", testutil.Attrs((*got)[0])["chat_id"].String())
	applied := migratetest.Applied(t, h)[stepNumber]
	require.Len(t, applied, 1)
	assert.Equal(t, int64(1), applied[0]["skipped_chats"])
	assert.Equal(t, int64(len(convert(t, chatA(), nobody).Skipped)), applied[0]["skipped_rows"])
	note := migratetest.Notes(t, h)[stepNumber][skippedKey("chat-b")]
	assert.Contains(t, note.Detail, "runner")
	assert.Equal(t, migrate.Counts{legacyRecords: records(broken), "converter": converterVersion}, note.Counts)
	for path, stamp := range before {
		if filepath.Base(path) != ledgerstore.FileName && !strings.HasPrefix(filepath.Base(path), ledgerstore.FileName) {
			assert.Equal(t, stamp, files(t, h.Store())[path], path)
		}
	}
}

// One row the reader cannot read, in any legacy table, keeps only its chat
// unconverted, and the warning names the row and the column.
func TestApply_ARowItCannotReadSkipsOnlyItsChat(t *testing.T) {
	for _, tc := range []struct{ db, table, column string }{
		{RunnerDB, "agent_chat_conversations", "first_seen_at"},
		{ActivityDB, "agent_turns", "started_at"},
		{ActivityDB, "agent_tool_calls", "duration_ms"},
		{ActivityDB, "agent_subagents", "ended_at"},
		{ActivityDB, "agent_interruptions", "display_order"},
		{ActivityDB, "agent_choices", "multi"},
	} {
		t.Run(tc.table, func(t *testing.T) {
			h := testutil.FixtureHome(t)
			writeLegacy(t, h, chatA(), chatB())
			spoil(t, h, tc.db, tc.table, tc.column, "chat-a", "garbled")
			log, got := capture(testutil.Message("chatledger: skipped a chat that cannot be converted"))

			require.NoError(t, bootLogging(t, h, log))

			assert.Empty(t, held(t, h, "chat-a"))
			assert.Equal(t, convert(t, chatB(), nobody).Rows, held(t, h, "chat-b"), "the other chat converts")
			require.Len(t, *got, 1)
			attrs := testutil.Attrs((*got)[0])
			assert.Equal(t, "chat-a", attrs["chat_id"].String())
			assert.Contains(t, attrs["err"].String(), tc.table)
			assert.Contains(t, attrs["err"].String(), tc.column+` "garbled"`)
		})
	}
}

// A legacy database that cannot be read is no chat to skip: the boot halts
// while planning, naming the step, and the file is left as it was.
func TestApply_ALegacyDatabaseThatCannotBeReadHalts(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/store/"+ActivityDB, "this is not a sqlite database, not even a little")

	err := bootLogging(t, h, slog.Default())

	var halt *migrate.Halt
	require.ErrorAs(t, err, &halt)
	assert.Equal(t, migrate.HaltPlanFailed, halt.Reason)
	raw, readErr := os.ReadFile(filepath.Join(h.Store(), ActivityDB))
	require.NoError(t, readErr)
	assert.Equal(t, "this is not a sqlite database, not even a little", string(raw))
}

// Verify runs as the step's own: a chat an earlier attempt marked done but
// left with open work fails it.
func TestImpl_VerifyFailsTheStepOnAChatLeftOpen(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatB())
	open, err := ledger.NewRow(ledger.KindUserMessage, "T-1", "", ledger.UserMessage{RequestID: "r1", Origin: ledger.OriginCrowbar})
	require.NoError(t, err)
	appendTo(t, h, "chat-b", open)
	testutil.WriteFile(t, h, "migrate/ledger.jsonl",
		`{"v":1,"kind":"run.start","run":"r1","pending":[6]}`+"\n"+
			`{"v":1,"kind":"checkpoint","run":"r1","step":6,"name":"chat-ledger","key":"chat-b","counts":{"rows":1}}`+"\n")

	err = bootLogging(t, h, slog.Default())

	var halt *migrate.Halt
	require.ErrorAs(t, err, &halt)
	assert.Equal(t, migrate.HaltVerifyFailed, halt.Reason)
}

// fakeLedger records the imports it is asked for; onImport runs inside each.
type fakeLedger struct {
	ownerErr error
	onImport func()
	imported []string
}

func (f *fakeLedger) SessionOwner(context.Context, string, string) (string, bool, error) {
	return "", false, f.ownerErr
}

func (f *fakeLedger) Import(_ context.Context, chatID string, _ []ledger.Row) error {
	f.imported = append(f.imported, chatID)
	if f.onImport != nil {
		f.onImport()
	}
	return nil
}

func (f *fakeLedger) Read(context.Context, string, int64, int) ([]ledger.Row, error) { return nil, nil }

type marks map[string]migrate.Counts

func (m marks) Settled(string) bool { return false }

func (m marks) Marked(key string) (migrate.Counts, bool) { n, ok := m[key]; return n, ok }

func (m marks) Mark(key string, n migrate.Counts) error { m[key] = n; return nil }

func (m marks) Noted(string) (migrate.Note, bool) { return migrate.Note{}, false }

func (m marks) Note(string, migrate.Note) error { return nil }

func TestConvertAll_StopsBeforeTheNextChatOnceCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &fakeLedger{onImport: cancel}
	cp := marks{}

	_, err := convertAll(ctx, slog.Default(), store, cp, legacyHome{chats: []LegacyChat{chatB(), chatA()}}, map[string]landed{})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"chat-b"}, store.imported, "no chat is imported after the cancel")
	assert.Contains(t, cp, "chat-b")
}

// A session index that cannot be read stops the step: read as "nobody owns
// it", the chat could take a session another chat holds.
func TestConvertAll_ASessionOwnerFailureStopsTheStep(t *testing.T) {
	broken := errors.New("session index unreadable")
	store := &fakeLedger{ownerErr: broken}
	cp := marks{}

	_, err := convertAll(context.Background(), slog.Default(), store, cp, legacyHome{chats: []LegacyChat{chatB()}}, map[string]landed{})

	require.ErrorIs(t, err, broken)
	require.NotErrorIs(t, err, errUnconvertible)
	assert.Empty(t, store.imported)
	assert.Empty(t, cp)
}
