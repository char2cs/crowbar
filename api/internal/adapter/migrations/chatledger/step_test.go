package chatledger

import (
	"context"
	"path/filepath"
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
	cat, err := migrate.NewCatalog(migrate.Step{Number: stepNumber, Name: "chat-ledger", Phase: migrate.PreOpen, Plan: Plan})
	require.NoError(t, err)
	sess, err := migrate.Begin(context.Background(), h, cat, migrate.Options{Version: "v-test"})
	require.NoError(t, err)
	require.NoError(t, sess.RunPreOpen(context.Background(), migrate.Impls{stepNumber: Impl()}))
	require.NoError(t, sess.Close())
}

// held is chatID's rows in h's ledger.
func held(t *testing.T, h home.Home, chatID string) []ledger.Row {
	t.Helper()
	var rows []ledger.Row
	require.NoError(t, withStore(h, func(s *ledgerstore.Store) error {
		var err error
		rows, err = s.Read(context.Background(), chatID, 0, 0)
		return err
	}))
	return rows
}

func appendTo(t *testing.T, h home.Home, chatID string, rows ...ledger.Row) {
	t.Helper()
	require.NoError(t, withStore(h, func(s *ledgerstore.Store) error {
		_, _, err := s.Append(context.Background(), chatID, rows...)
		return err
	}))
}

func TestApply_ConvertsEachChatOnceAndResumesFromItsCheckpoints(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA(), chatB())
	testutil.WriteFile(t, h, "migrate/ledger.jsonl",
		`{"v":1,"kind":"run.start","run":"r1","pending":[6]}`+"\n"+
			`{"v":1,"kind":"checkpoint","run":"r1","step":6,"name":"chat-ledger","key":"chat-b","counts":{"rows":3}}`+"\n")

	boot(t, h)

	want := convert(t, chatA(), nobody)
	assert.Equal(t, want.Rows, held(t, h, "chat-a"))
	assert.Empty(t, held(t, h, "chat-b"), "a chat an earlier attempt marked done is not converted again")
	applied := migratetest.Applied(t, h)[stepNumber]
	require.Len(t, applied, 1)
	assert.Equal(t, migrate.Counts{"rows": int64(len(want.Rows)) + 3, "skipped_rows": int64(len(want.Skipped))}, applied[0])

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
	require.NoError(t, withStore(h, func(s *ledgerstore.Store) error {
		return s.Import(context.Background(), "chat-a", want.Rows)
	}))

	boot(t, h)

	assert.Equal(t, want.Rows, held(t, h, "chat-a"))
	assert.Equal(t, []migrate.Counts{{"rows": int64(len(want.Rows)), "skipped_rows": int64(len(want.Skipped))}},
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

	require.ErrorIs(t, verify(context.Background(), h, map[string]map[string]bool{"chat-x": nil}), ErrNotAtRest)
	require.ErrorIs(t, verify(context.Background(), h, map[string]map[string]bool{"chat-y": nil}), ErrNotAtRest)
	require.NoError(t, verify(context.Background(), h, map[string]map[string]bool{"chat-y": {"run-1": true}}),
		"a runner the old daemon left is the boot reconcile's to end")
}
