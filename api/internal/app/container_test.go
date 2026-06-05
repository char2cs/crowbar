package app_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app"
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
}

func TestApp_New_NilWorkspaceES_ReturnsError(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	adapters.WorkspaceES = nil

	_, err = app.New(context.Background(), newEng(t), adapters)
	assert.Error(t, err)
}

func TestApp_New_NilChatES_ReturnsError(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	adapters.ChatES = nil

	_, err = app.New(context.Background(), newEng(t), adapters)
	assert.Error(t, err)
}

func TestApp_New_NilAgentRunES_ReturnsError(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	adapters.AgentRunES = nil

	_, err = app.New(context.Background(), newEng(t), adapters)
	assert.Error(t, err)
}

func TestApp_New_NilReviewThreadES_ReturnsError(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	adapters.ReviewThreadES = nil

	_, err = app.New(context.Background(), newEng(t), adapters)
	assert.Error(t, err)
}

func TestApp_New_ClosedDB_ReturnsError(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	sqlDB, err := adapters.DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = app.New(context.Background(), newEng(t), adapters)
	assert.Error(t, err)
}
