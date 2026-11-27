package chatledger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
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

func TestBoot_AChatAnOlderBuildAddedAfterTheUpgradeIsConverted(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA())
	boot(t, h)
	writeLegacy(t, h, chatB())

	boot(t, h)

	assert.Equal(t, convert(t, chatA(), nobody).Rows, held(t, h, "chat-a"))
	assert.Equal(t, convert(t, chatB(), nobody).Rows, held(t, h, "chat-b"))
	assert.Len(t, migratetest.Applied(t, h)[stepNumber], 2)
	backups, err := filepath.Glob(filepath.Join(h.MigrateDir(), "backups", "*.tar.gz"))
	require.NoError(t, err)
	assert.Len(t, backups, 1, "a later run only adds chats the ledger lacks: the upgrade's backup is the only one")
	before := testutil.TreeSnapshot(t, h)
	boot(t, h)
	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
}

func TestBoot_ALegacyFileDamagedAfterTheUpgradeIsAWarningNotAHalt(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA())
	boot(t, h)
	rows := held(t, h, "chat-a")
	testutil.WriteFile(t, h, "state/store/"+ActivityDB, "this is not a sqlite database, not even a little")
	before := testutil.TreeSnapshot(t, h)
	log, warned := capture(testutil.AtLeast(slog.LevelWarn))

	require.NoError(t, bootLogging(t, h, log))

	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
	assert.Equal(t, rows, held(t, h, "chat-a"))
	require.Len(t, *warned, 1)
	assert.Contains(t, testutil.Attrs((*warned)[0])["err"].String(), "not a database")
}

func TestBoot_ASkippedChatIsNotTriedAtEveryBoot(t *testing.T) {
	h := testutil.FixtureHome(t)
	broken := chatB()
	broken.Runners = []LegacyRunner{{ID: "", ChatID: broken.ChatID, ProviderID: "codex"}}
	writeLegacy(t, h, chatA(), broken)
	boot(t, h)
	before := testutil.TreeSnapshot(t, h)

	boot(t, h)

	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
}

func TestPlan_LegacyFilesAsTheLastRunLeftThemAreOnlyStatted(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA())
	boot(t, h)
	notes := migratetest.Notes(t, h)[stepNumber]
	require.Contains(t, notes, stampKey)

	plan, err := Plan(t.Context(), migrate.Env{Home: h, Notes: notes})
	require.NoError(t, err)
	assert.True(t, plan.Empty())

	testutil.WriteFile(t, h, "state/store/"+ActivityDB, "not a database any more")
	_, err = Plan(t.Context(), migrate.Env{Home: h, Notes: notes})
	require.Error(t, err, "a changed stamp is counted again")
}

func TestBoot_ASkippedChatIsRetriedOnceTheConverterChanges(t *testing.T) {
	h := testutil.FixtureHome(t)
	broken := chatB()
	broken.Runners = []LegacyRunner{{ID: "", ChatID: broken.ChatID, ProviderID: "codex"}}
	writeLegacy(t, h, broken)
	boot(t, h)
	path := filepath.Join(h.MigrateDir(), "ledger.jsonl")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	now := strconv.Itoa(converterVersion)
	older := strings.ReplaceAll(strings.ReplaceAll(string(raw),
		`"converter":`+now, `"converter":0`), `converter `+now, `converter 0`)
	require.NoError(t, os.WriteFile(path, []byte(older), 0o600))
	log, warned := capture(testutil.Message("chatledger: skipped a chat that cannot be converted"))

	require.NoError(t, bootLogging(t, h, log))
	require.NoError(t, bootLogging(t, h, log))

	assert.Len(t, *warned, 1, "retried by the newer converter, then not again")
	assert.Equal(t, int64(converterVersion), migratetest.Notes(t, h)[stepNumber][skippedKey("chat-b")].Counts["converter"])
}

func records(chat LegacyChat) int64 {
	return int64(len(chat.Conversations) + len(chat.Turns) + len(chat.Tools) + len(chat.Subagents) +
		len(chat.Interruptions) + len(chat.Choices) + len(chat.Prompts) + len(chat.Unreadable))
}

func TestBoot_AHugeDamagedCellIsNamedShort(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA(), chatB())
	huge := strings.Repeat("x", 20<<20)
	spoil(t, h, ActivityDB, "agent_turns", "seq", "chat-a", huge)
	spoil(t, h, ActivityDB, "agent_turns", "role", "chat-b", huge)
	spoil(t, h, RunnerDB, "agent_chat_conversations", "first_seen_at", "chat-b", huge)
	log, warned := capture(testutil.AtLeast(slog.LevelWarn))

	require.NoError(t, bootLogging(t, h, log))

	for _, r := range *warned {
		for k, v := range testutil.Attrs(r) {
			assert.Less(t, len(fmt.Sprint(v.Any())), 4096, "%s of %q", k, r.Message)
		}
	}
	info, err := os.Stat(filepath.Join(h.MigrateDir(), "ledger.jsonl"))
	require.NoError(t, err)
	assert.Less(t, info.Size(), int64(64<<10))
	named := slices.ContainsFunc(*warned, func(r slog.Record) bool {
		return strings.Contains(testutil.Attrs(r)["skipped"].String(), "…(20971520 bytes)")
	})
	assert.True(t, named, "the cell is named by its first bytes and its length")
}

func killAfterStamp(t *testing.T, h home.Home) {
	t.Helper()
	path := filepath.Join(h.MigrateDir(), "ledger.jsonl")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.SplitAfter(string(raw), "\n")
	last := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], `"key":"`+stampKey+`"`) {
			last = i
			break
		}
	}
	require.Positive(t, last)
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines[:last+1], "")), 0o600))
}

func TestBoot_AChatWrittenToAfterAKilledRunNeverHaltsALaterOne(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA())
	boot(t, h)
	writeLegacy(t, h, chatB())
	boot(t, h)
	killAfterStamp(t, h)
	boot(t, h)
	r, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerStopped})
	require.NoError(t, err)
	appendTo(t, h, "chat-b", r)
	later := chatB()
	later.ChatID = "chat-c"
	later.Conversations = nil
	for i := range later.Turns {
		later.Turns[i].ChatID = "chat-c"
	}
	later.Interruptions = nil
	writeLegacy(t, h, later)

	require.NoError(t, bootLogging(t, h, slog.Default()))

	assert.Len(t, held(t, h, "chat-b"), len(convert(t, chatB(), nobody).Rows)+1)
	assert.NotEmpty(t, held(t, h, "chat-c"))
}

func refuse(t *testing.T, h home.Home, chatID string) {
	t.Helper()
	require.NoError(t, withStore(t.Context(), h, func(*ledgerstore.Store) error { return nil }))
	conn := writable(t, filepath.Join(h.Store(), ledgerstore.FileName))
	when := ""
	if chatID != "" {
		when = "WHEN NEW.chat_id = '" + chatID + "' "
	}
	_, err := conn.ExecContext(t.Context(), "CREATE TRIGGER refuse BEFORE INSERT ON chat_ledger "+when+
		"BEGIN SELECT RAISE(ABORT, 'refused'); END")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func allow(t *testing.T, h home.Home) {
	t.Helper()
	conn := writable(t, filepath.Join(h.Store(), ledgerstore.FileName))
	_, err := conn.ExecContext(t.Context(), "DROP TRIGGER refuse")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func TestBoot_AChatWhoseImportFailedIsTriedAgainAtTheNextBoot(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA(), chatB())
	refuse(t, h, "chat-b")
	boot(t, h)
	require.Empty(t, held(t, h, "chat-b"))
	allow(t, h)

	boot(t, h)

	assert.Equal(t, convert(t, chatA(), nobody).Rows, held(t, h, "chat-a"))
	assert.Equal(t, convert(t, chatB(), nobody).Rows, held(t, h, "chat-b"))
	statsOnly(t, h)
}

func statsOnly(t *testing.T, h home.Home) {
	t.Helper()
	testutil.RequirePermissionEnforcement(t)
	for _, db := range []string{ActivityDB, RunnerDB} {
		path := filepath.Join(h.Store(), db)
		require.NoError(t, os.Chmod(path, 0))
		t.Cleanup(func() { require.NoError(t, os.Chmod(path, 0o600)) })
	}
	log, warned := capture(testutil.AtLeast(slog.LevelWarn))
	require.NoError(t, bootLogging(t, h, log))
	assert.Empty(t, *warned, "the legacy records were read, not only statted")
}

func TestBoot_AChatWaitingForARetryThatLeavesTheLegacyFilesIsOwedNothing(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA(), chatB())
	refuse(t, h, "chat-b")
	boot(t, h)
	for db, tables := range map[string][]string{
		RunnerDB:   {"agent_chat_conversations"},
		ActivityDB: {"agent_turns", "agent_interruptions"},
	} {
		conn := writable(t, filepath.Join(h.Store(), db))
		for _, table := range tables {
			_, err := conn.ExecContext(t.Context(), "DELETE FROM "+table+" WHERE chat_id = 'chat-b'")
			require.NoError(t, err)
		}
		require.NoError(t, conn.Close())
	}
	boot(t, h)

	statsOnly(t, h)
}

func TestBoot_AChatWhoseImportKeepsFailingIsGivenUp(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA(), chatB())
	refuse(t, h, "chat-b")
	log, tried := capture(func(r slog.Record) bool {
		return strings.HasPrefix(r.Message, "chatledger: skipped a chat whose import failed")
	})

	for range importAttempts + 2 {
		require.NoError(t, bootLogging(t, h, log))
	}

	require.Len(t, *tried, importAttempts)
	assert.Contains(t, (*tried)[0].Message, "the next boot tries it again")
	assert.Contains(t, (*tried)[importAttempts-1].Message, "not tried again")
	assert.Equal(t, slog.LevelError, (*tried)[importAttempts-1].Level, "a chat given up is said once, as an error")
	note := migratetest.Notes(t, h)[stepNumber][skippedKey("chat-b")]
	assert.Equal(t, int64(converterVersion), note.Counts["converter"])
	assert.Equal(t, int64(importAttempts), note.Counts["attempts"])
	assert.Contains(t, note.Detail, "refused")
	assert.Empty(t, held(t, h, "chat-b"))
	before := testutil.TreeSnapshot(t, h)
	boot(t, h)
	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
}

func TestBoot_ALedgerRefusingEveryImportGivesUpNoChat(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA(), chatB())
	refuse(t, h, "")
	for range importAttempts + 1 {
		require.Error(t, bootLogging(t, h, slog.Default()))
	}
	allow(t, h)

	require.NoError(t, bootLogging(t, h, slog.Default()))

	assert.Equal(t, convert(t, chatA(), nobody).Rows, held(t, h, "chat-a"))
	assert.Equal(t, convert(t, chatB(), nobody).Rows, held(t, h, "chat-b"))
}

func TestBoot_AChatWrittenToWhileItsImportWaitsSaysItsHistoryWasNotImported(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA(), chatB())
	refuse(t, h, "chat-b")
	boot(t, h)
	allow(t, h)
	r, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerStopped})
	require.NoError(t, err)
	appendTo(t, h, "chat-b", r)

	boot(t, h)

	rows := held(t, h, "chat-b")
	require.Len(t, rows, 2, "what was written, then the notice")
	assert.Equal(t, ledger.Marker{Kind: ledger.MarkerUnreadable, Detail: historyNotImported}, body[ledger.Marker](t, rows[1]))
	note := migratetest.Notes(t, h)[stepNumber][skippedKey("chat-b")]
	assert.Equal(t, int64(converterVersion), note.Counts["converter"])
	before := testutil.TreeSnapshot(t, h)
	boot(t, h)
	assert.Equal(t, before, testutil.TreeSnapshot(t, h), "said once")
}

type notes struct {
	marks
	notes map[string]migrate.Note
}

func (n notes) Noted(key string) (migrate.Note, bool) { v, ok := n.notes[key]; return v, ok }

func (n notes) Note(key string, v migrate.Note) error { n.notes[key] = v; return nil }

func TestSettled_OnlyARunThatLeftNoChatOutsideTheLedger(t *testing.T) {
	ran := map[string]migrate.Note{stampKey: {Detail: "converter 5\n"}}
	tests := []struct {
		name  string
		marks migrate.Marks
		notes map[string]migrate.Note
		want  bool
	}{
		{name: "never ran", want: false},
		{name: "ran and owes nothing", notes: ran, want: true},
		{name: "owes a retry", notes: map[string]migrate.Note{stampKey: {Counts: migrate.Counts{retryingChats: 1}}}, want: false},
		{
			name:  "skipped a chat",
			notes: map[string]migrate.Note{stampKey: {}, skippedKey("c1"): {Detail: "unconvertible"}},
			want:  false,
		},
		{
			name:  "skipped a chat a later converter converted",
			marks: migrate.Marks{"c1": {legacyRecords: 2}},
			notes: map[string]migrate.Note{stampKey: {}, skippedKey("c1"): {Detail: "unconvertible"}},
			want:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Settled(tc.marks, tc.notes))
		})
	}
}
