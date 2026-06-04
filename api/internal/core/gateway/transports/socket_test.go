package transports

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSocket_ExplicitPath(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "x.sock")
	l, err := NewSocket("unix://" + sockPath)
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	assert.NotNil(t, l)
}

func TestNewSocket_DefaultPath_UsesHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	l, err := NewSocket("unix://")
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	assert.NotNil(t, l)
}

func TestSocketPath_NonEmpty(t *testing.T) {
	path, err := socketPath("/tmp/some.sock")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/some.sock", path)
}

func TestSocketPath_EmptyUsesHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := socketPath("")
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.Contains(t, path, defaultSocketName)
}
