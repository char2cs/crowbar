package chatledger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

// An older build booted on an upgraded home keeps writing the legacy tables.
// The next boot converts each chat the ledger lacks, without another backup,
// and a boot that finds nothing owed writes nothing at all.
func TestBoot_AChatAnOlderBuildAddedAfterTheUpgradeIsConverted(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA())
	boot(t, h)
	writeLegacy(t, h, chatB())

	boot(t, h)

	assert.Equal(t, convert(t, chatA(), nobody).Rows, held(t, h, "chat-a"))
	assert.Equal(t, convert(t, chatB(), nobody).Rows, held(t, h, "chat-b"))
	assert.Len(t, migratetest.Applied(t, h)[stepNumber], 2)
	backups, err := os.ReadDir(filepath.Join(h.MigrateDir(), "backups"))
	require.NoError(t, err)
	assert.Len(t, backups, 1, "a later run only adds chats the ledger lacks: the upgrade's backup is the only one")
	before := testutil.TreeSnapshot(t, h)
	boot(t, h)
	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
}

// Rows an older build added to a chat already converted are never imported
// over what the ledger holds since; the loss is noted and warned of once.
func TestBoot_AConvertedChatAnOlderBuildAddedToIsRecordedOnce(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA())
	boot(t, h)
	rows := held(t, h, "chat-a")
	more := LegacyChat{ChatID: "chat-a", Turns: []LegacyTurn{{
		ID: "u-late", ChatID: "chat-a", Role: roleUser, Text: "typed on the older build", Seq: 99, StartedAt: at(12, 0, 0),
	}}}
	writeLegacy(t, h, more)
	log, warned := capture(testutil.AtLeast(slog.LevelWarn))

	require.NoError(t, bootLogging(t, h, log))

	assert.Equal(t, rows, held(t, h, "chat-a"))
	require.Len(t, *warned, 1)
	attrs := testutil.Attrs((*warned)[0])
	assert.Equal(t, "chat-a", attrs["chat_id"].String())
	assert.Equal(t, records(chatA())+1, attrs["legacy_records"].Int64())
	assert.Equal(t, records(chatA()), attrs["converted_records"].Int64())
	before := testutil.TreeSnapshot(t, h)
	require.NoError(t, bootLogging(t, h, log))
	assert.Len(t, *warned, 1, "the same loss is warned of once")
	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
}

// Legacy files damaged after the upgrade cannot stop a home that already runs
// on the ledger: the boot warns, writes nothing, and the ledger is as it was.
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

// A chat skipped once is noted, so later boots neither retry it nor run the step.
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

// Legacy files the last run saw unchanged are not even counted: the plan
// only stats them. Once they change, one run records what it found.
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

// A skipped chat is tried again by the first build whose converter is newer
// than the one that skipped it, and only then.
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

// records is how many legacy records chat holds, as census counts them.
func records(chat LegacyChat) int64 {
	return int64(len(chat.Conversations) + len(chat.Turns) + len(chat.Tools) + len(chat.Subagents) +
		len(chat.Interruptions) + len(chat.Choices) + len(chat.Prompts) + len(chat.Unreadable))
}

// A damaged cell of any size is named by its first bytes and its length: the
// warning, the note and the run ledger stay small.
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
	assert.Contains(t, migratetest.Notes(t, h)[stepNumber][skippedKey("chat-b")].Detail, "…(20971520 bytes)")
}

// killAfterStamp leaves the run ledger as a process killed between the last
// run's legacy-files note and its step.done would have.
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

// A chat imported by a run killed before its step.done, then written to by
// normal use, never halts a later run: what that run checks is what it wrote.
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
