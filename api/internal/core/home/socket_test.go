package home

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSocket_DefaultHomeBindsInsideTheHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME does not set the user home on windows")
	}
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv(EnvVar, "")

	h, err := Resolve()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(userHome, ".crowbar", "crowbar.sock"), h.Socket())
}

func TestSocket_SpelledOutDefaultIsNotAnOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME does not set the user home on windows")
	}
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv(EnvVar, filepath.Join(userHome, ".crowbar"))

	h, err := Resolve()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(userHome, ".crowbar", "crowbar.sock"), h.Socket())
}

func TestSocket_OverrideLivesInTempKeyedByTheHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvVar, root)

	h, err := Resolve()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(os.TempDir(), socketName(root)), h.Socket())
}

func TestSocketName_IsStable(t *testing.T) {
	assert.Equal(t, "crowbar-c13f09536446a88e.sock", socketName("/dev/crowbar-home"))
}
