package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestCatalog_IsValid(t *testing.T) {
	cat, err := Catalog()

	require.NoError(t, err)
	assert.Equal(t, 0, cat.Known(), "the legacy steps change no layout")
}

func TestPlan_MarkerPresentIsAnAdoptionNotAWork(t *testing.T) {
	h := testutil.FixtureHome(t)

	assert.Equal(t, migrate.Plan{
		Actions:       []migrate.Action{{Kind: migrate.Convert, Subject: "chats minted before chat types existed", Count: migrate.Unknown}},
		RewritesState: true,
	}, planOf(t, h, StepChatTypes))

	testutil.WriteFile(t, h, "state/upgrades/chat-types", "")
	assert.Equal(t, migrate.Plan{
		Actions: []migrate.Action{{Kind: migrate.Delete, Subject: "state/upgrades/chat-types", Count: 1}},
	}, planOf(t, h, StepChatTypes))
}

func TestPlan_HookDeliveriesNeverRewritesState(t *testing.T) {
	h := testutil.FixtureHome(t)

	assert.False(t, planOf(t, h, StepHookDeliveries).RewritesState)
	testutil.WriteFile(t, h, "state/upgrades/hook-deliveries", "")
	assert.False(t, planOf(t, h, StepHookDeliveries).RewritesState)
}

func TestPlan_WorkspacePathsTableHasNothingToDoWithoutAViewDB(t *testing.T) {
	h := testutil.FixtureHome(t)

	assert.True(t, planOf(t, h, StepWorkspacePathsTable).Empty())

	testutil.WriteFile(t, h, "state/view.db", "")
	assert.Equal(t, migrate.Plan{
		Actions:       []migrate.Action{{Kind: migrate.Convert, Subject: "state/view.db: drop table workspace_paths", Count: migrate.Unknown}},
		RewritesState: true,
	}, planOf(t, h, StepWorkspacePathsTable))
}

// A home this binary must not migrate is not an error: the session is live,
// holds the home, and says why, so the daemon can report it.
func TestBegin_ARefusedHomeIsALiveHaltedSession(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "LAYOUT", "9\n")

	sess, err := Begin(context.Background(), h, migrate.Options{Version: "v-test"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sess.Close()) })

	require.NotNil(t, sess.Halt())
	assert.Equal(t, migrate.HaltLayoutNewer, sess.Halt().Reason)
	var halt *migrate.Halt
	require.ErrorAs(t, sess.RunPreOpen(context.Background(), PreOpenImpls()), &halt)
	assert.NoDirExists(t, h.MigrateDir())
}

func planOf(
	t *testing.T,
	h home.Home,
	number int,
) migrate.Plan {
	t.Helper()
	for _, step := range legacySteps() {
		if step.Number == number {
			plan, err := step.Plan(context.Background(), migrate.Env{Home: h})
			require.NoError(t, err)
			return plan
		}
	}
	t.Fatalf("no legacy step %d", number)
	return migrate.Plan{}
}
