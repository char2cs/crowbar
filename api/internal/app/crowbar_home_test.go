package app_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app"
)

func TestApp_CrowbarHome_IsTheHomeTheAdapterOpened(t *testing.T) {
	root := t.TempDir()
	adapters, err := adapter.New(adapter.WithHomeDir(root))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(context.Background(), newEng(t), adapters)
	require.NoError(t, err)

	assert.Equal(t, root, c.CrowbarHome())
}
