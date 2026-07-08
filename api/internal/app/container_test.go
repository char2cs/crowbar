package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/engine"
)

func newEng(t *testing.T) *engine.Container {
	t.Helper()
	eng, err := engine.New(context.Background())
	require.NoError(t, err)
	return eng
}

func TestApp_New_BootsFullLayer(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters)
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

	c, err := app.New(ctx, newEng(t), adapters)
	require.NoError(t, err)

	_, err = c.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b"},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)

	// The store projection is async (Send, not SendWait): the read model catches
	// up shortly after Create returns.
	require.Eventually(t, func() bool {
		rows, listErr := c.Usecases.Workspace.List(ctx)
		return listErr == nil && len(rows) == 1 && rows[0].ID == "w1"
	}, 2*time.Second, 5*time.Millisecond)
}
