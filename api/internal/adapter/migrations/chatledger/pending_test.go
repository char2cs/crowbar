package chatledger

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

// An older build booted on an upgraded home keeps writing the legacy tables.
// The next boot converts each chat the ledger lacks, and a boot that finds
// nothing owed writes nothing at all.
func TestBoot_AChatAnOlderBuildAddedAfterTheUpgradeIsConverted(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA())
	boot(t, h)
	writeLegacy(t, h, chatB())

	boot(t, h)

	assert.Equal(t, convert(t, chatA(), nobody).Rows, held(t, h, "chat-a"))
	assert.Equal(t, convert(t, chatB(), nobody).Rows, held(t, h, "chat-b"))
	assert.Len(t, migratetest.Applied(t, h)[stepNumber], 2)
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
