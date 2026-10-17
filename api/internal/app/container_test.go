package app_test

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

func newEng(t *testing.T) *engine.Container {
	t.Helper()
	eng, err := engine.New(context.Background())
	require.NoError(t, err)
	t.Cleanup(eng.Close)
	return eng
}

func TestApp_New_BootsFullLayer(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters, migratetest.NoPostOpen())
	require.NoError(t, err)
	assert.NotNil(t, c.Hub)
	assert.NotNil(t, c.Repositories)
	assert.NotNil(t, c.GORM)
	assert.NotNil(t, c.Repositories.Workspace)
	assert.NotNil(t, c.Usecases)
	assert.NotNil(t, c.Usecases.Workspace)
}

func TestApp_New_UsecasesWorkspaceListEndToEnd(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters, migratetest.NoPostOpen())
	require.NoError(t, err)

	_, err = c.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b", Provisioning: domain.WorkspacePlaceholder},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)

	// The store projection is async (Send, not SendWait): drain every asynx
	// instance so the read model has settled, then read it synchronously — the
	// deterministic read-your-writes barrier, with no polling and no timeout.
	c.Repositories.WaitQuiescent()
	rows, err := c.Usecases.Workspace.List(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "w1", rows[0].ID)
}

// recordingRunner is a post-open runner that notes which steps it was handed
// and returns err.
type recordingRunner struct {
	steps []int
	err   error
}

func (r *recordingRunner) RunPostOpen(
	_ context.Context,
	impls migrate.Impls,
) error {
	r.steps = slices.Sorted(maps.Keys(impls))
	return r.err
}

func TestApp_New_HandsEveryPostOpenStepToTheRunner(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	runner := &recordingRunner{}

	_, err = app.New(context.Background(), newEng(t), adapters, runner)

	require.NoError(t, err)
	assert.Equal(t, []int{
		migrations.StepProjectHomes, migrations.StepChatTypes, migrations.StepChatProviders, migrations.StepHookDeliveries,
	}, runner.steps)
}

func TestApp_New_AHaltedMigrationFailsTheBoot(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	halt := &migrate.Halt{Reason: migrate.HaltStepFailed, Message: "step 3 failed"}

	_, err = app.New(context.Background(), newEng(t), adapters, &recordingRunner{err: halt})

	var got *migrate.Halt
	require.ErrorAs(t, err, &got)
	assert.Same(t, halt, got)
}
