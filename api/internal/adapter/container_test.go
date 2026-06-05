package adapter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
)

func TestNew_BootsAllStores(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	assert.NotNil(t, c.WorkspaceES)
	assert.NotNil(t, c.ChatES)
	assert.NotNil(t, c.AgentRunES)
	assert.NotNil(t, c.ReviewThreadES)
	assert.NotNil(t, c.DB)
}

func TestClose_Idempotentish(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	assert.NoError(t, c.Close())
}

func TestNew_DefaultDirs_UsesHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c, err := adapter.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	assert.NotNil(t, c.WorkspaceES)
}
