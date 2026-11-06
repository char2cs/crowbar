package migrations

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations/chatledger"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

// legacyPaths are the files legacyHome leaves that the retire step owns.
var legacyPaths = []string{
	"state/store/" + chatledger.ActivityDB,
	"state/store/" + chatledger.RunnerDB,
	"state/events/node.db",
	"state/events/agent_activity_snapshots.db",
	"state/upgrades/chat-types-done-long-ago",
	"chats/c1/" + chatledger.PromptsDir + "/req-1.json",
	"bin/crowbar",
}

// legacyHome is a pre-ledger home: one chat known only by its prompt journal,
// the old databases beside it, a stray marker and the old binary copy.
func legacyHome(
	t *testing.T,
) home.Home {
	t.Helper()
	h := testutil.FixtureHome(t)
	prompt, err := json.Marshal(chatledger.LegacyPrompt{
		RequestID: "req-1", Text: "hello", State: "accepted", ProviderID: "p",
		CreatedAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 10, 1, 9, 0, 1, 0, time.UTC),
	})
	require.NoError(t, err)
	for _, rel := range legacyPaths {
		testutil.WriteFile(t, h, rel, "legacy bytes of "+filepath.Base(rel))
	}
	testutil.WriteFile(t, h, "state/store/"+chatledger.ActivityDB, "")
	testutil.WriteFile(t, h, "state/store/"+chatledger.RunnerDB, "")
	testutil.WriteFile(t, h, "chats/c1/"+chatledger.PromptsDir+"/req-1.json", string(prompt))
	return h
}

// bootLedgerSteps boots h through the catalog's chat-ledger, layout and retire
// steps alone, as the daemon's pre-open half runs them.
func bootLedgerSteps(
	t *testing.T,
	h home.Home,
) {
	t.Helper()
	keep := []int{StepChatLedger, StepLedgerLayout, StepRetireLegacy}
	steps := slices.DeleteFunc(legacySteps(), func(s migrate.Step) bool { return !slices.Contains(keep, s.Number) })
	cat, err := migrate.NewCatalog(steps...)
	require.NoError(t, err)
	impls := migrate.Impls{}
	for _, n := range keep {
		impls[n] = PreOpenImpls()[n]
	}
	sess, err := migrate.Begin(context.Background(), h, cat, migrate.Options{Version: "v-test"})
	require.NoError(t, err)
	require.NoError(t, sess.RunPreOpen(context.Background(), impls))
	require.NoError(t, sess.RunPostOpen(context.Background(), impls))
	require.NoError(t, sess.Close())
}

func exists(
	h home.Home,
	rel string,
) bool {
	_, err := os.Lstat(filepath.Join(h.Root(), filepath.FromSlash(rel)))
	return err == nil
}

func TestRetireLegacy_DeletesTheLegacyFilesOnTheBootAfterTheMigratedHomeBooted(t *testing.T) {
	h := legacyHome(t)

	bootLedgerSteps(t, h)

	for _, rel := range legacyPaths {
		assert.True(t, exists(h, rel), "the migrating boot deletes nothing: %s", rel)
	}
	layout, err := os.ReadFile(h.LayoutFile())
	require.NoError(t, err)
	assert.Equal(t, "1\n", string(layout), "a migrated home says its layout")

	bootLedgerSteps(t, h)

	for _, rel := range legacyPaths {
		assert.False(t, exists(h, rel), "the boot after deletes %s", rel)
	}
	for _, dir := range []string{"chats", "state/upgrades", "bin"} {
		assert.False(t, exists(h, dir), "an emptied legacy directory goes too: %s", dir)
	}
	assert.True(t, exists(h, "state/store/chat_ledger.db"))
	retired := migratetest.Applied(t, h)[StepRetireLegacy]
	require.Len(t, retired, 2)
	assert.Equal(t, int64(len(legacyPaths)), retired[1]["deleted_files"])

	before := len(migratetest.Applied(t, h)[StepRetireLegacy])
	bootLedgerSteps(t, h)
	assert.Len(t, migratetest.Applied(t, h)[StepRetireLegacy], before, "with nothing left the step does not run again")
}

// A legacy file written after the backup was taken is not in it as it is: it
// stays, and is not looked at again until it changes.
func TestRetireLegacy_KeepsAFileTheBackupDoesNotHoldAsItIs(t *testing.T) {
	h := legacyHome(t)
	bootLedgerSteps(t, h)
	testutil.WriteFile(t, h, "state/events/node.db", "written by an older build after the upgrade")

	bootLedgerSteps(t, h)

	assert.True(t, exists(h, "state/events/node.db"))
	assert.False(t, exists(h, "state/store/"+chatledger.ActivityDB))
	retired := migratetest.Applied(t, h)[StepRetireLegacy]
	assert.Equal(t, int64(1), retired[len(retired)-1]["kept_files"])
	assert.Contains(t, migratetest.Notes(t, h)[StepRetireLegacy], "state/events/node.db")

	bootLedgerSteps(t, h)
	assert.Len(t, migratetest.Applied(t, h)[StepRetireLegacy], len(retired), "an unchanged kept file is not hashed again")
}

// The engine halts a build whose catalog stops at layout 0 on a migrated home,
// so it can never re-create the retired files empty.
func TestLedgerLayout_ABuildThatKnowsOnlyLayoutZeroHaltsOnAMigratedHome(t *testing.T) {
	h := legacyHome(t)
	bootLedgerSteps(t, h)
	older, err := migrate.NewCatalog(migrate.Step{Number: StepChatLedger, Name: "chat-ledger", Phase: migrate.PreOpen, Plan: chatledger.Plan, Recurs: true})
	require.NoError(t, err)
	before := testutil.TreeSnapshot(t, h)

	sess, err := migrate.Begin(context.Background(), h, older, migrate.Options{Version: "older"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sess.Close()) })

	require.NotNil(t, sess.Halt())
	assert.Equal(t, migrate.HaltLayoutNewer, sess.Halt().Reason)
	assert.Contains(t, sess.Halt().Message, "run a newer Crowbar")
	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
}
