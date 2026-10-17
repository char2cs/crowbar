package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// migratedHome is a fixture home a daemon has booted once: every legacy step
// is recorded as applied, and the seeder may open it.
func migratedHome(t *testing.T) home.Home {
	t.Helper()
	ctx := context.Background()
	h := testutil.FixtureHome(t)
	session, err := migrations.Begin(ctx, h, migrate.Options{Version: "v-test"})
	require.NoError(t, err)
	impls := migrations.PreOpenImpls()
	for _, number := range []int{
		migrations.StepProjectHomes, migrations.StepChatTypes,
		migrations.StepChatProviders, migrations.StepHookDeliveries,
	} {
		impls[number] = migrate.Impl{Apply: func(context.Context, migrate.Env, *migrate.Checkpoints) (migrate.Counts, error) {
			return nil, nil
		}}
	}
	require.NoError(t, session.RunPreOpen(ctx, impls))
	require.NoError(t, session.RunPostOpen(ctx, impls))
	require.NoError(t, session.Close())
	return h
}

func seedFrom(t *testing.T, h home.Home) (string, error) {
	t.Helper()
	t.Setenv(home.EnvVar, h.Root())
	return run(context.Background(), seedOptions{WorkspaceID: "ws-1", ChatID: "seeded", Turns: 1, ToolCallsPerTurn: 1})
}

func TestRun_RefusesAHomeWithPendingStepsAndWritesNothing(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/upgrades/chat-types", "")
	before := testutil.TreeSnapshot(t, h)

	_, err := seedFrom(t, h)

	require.ErrorContains(t, err, "start the daemon once to finish migrating this home")
	assert.Equal(t, before, testutil.TreeSnapshot(t, h), "no lock file, run, backup or store is left behind")
	assert.NoDirExists(t, h.MigrateDir())
}

func TestRun_RefusesAHomeThisBuildMustNotMigrate(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "LAYOUT", "9\n")
	before := testutil.TreeSnapshot(t, h)

	_, err := seedFrom(t, h)

	var halt *migrate.Halt
	require.ErrorAs(t, err, &halt)
	assert.Equal(t, migrate.HaltLayoutNewer, halt.Reason)
	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
}

func TestRun_SeedsAMigratedHome(t *testing.T) {
	h := migratedHome(t)

	chatID, err := seedFrom(t, h)

	require.NoError(t, err)
	assert.Equal(t, "seeded", chatID)
	session, err := migrations.Begin(context.Background(), h, migrate.Options{})
	require.NoError(t, err, "the seeder lets go of the home")
	require.NoError(t, session.Close())
}

func TestRun_RefusesAHomeAnotherProcessHolds(t *testing.T) {
	h := migratedHome(t)
	holder, err := migrations.Begin(context.Background(), h, migrate.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, holder.Close()) })

	_, err = seedFrom(t, h)

	require.ErrorIs(t, err, migrate.ErrHomeLocked)
}
