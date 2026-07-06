package selfinstall_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/selfinstall"
	"github.com/stretchr/testify/require"
)

func TestInstall_CopiesExecutableIntoBin(t *testing.T) {
	home := t.TempDir()
	got, err := selfinstall.Install(home)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, "bin", "crowbar"), got)

	info, err := os.Stat(got)
	require.NoError(t, err)
	require.NotZero(t, info.Size())
	require.NotZero(t, info.Mode()&0o100, "installed binary must be executable")
}

// TestInstall_IdempotentSkipsRecopyWhenSameSize covers the "already installed,
// same size" fast path: a second Install call must not touch a destination
// that already has the right size, even if its content has since changed.
func TestInstall_IdempotentSkipsRecopyWhenSameSize(t *testing.T) {
	home := t.TempDir()
	got, err := selfinstall.Install(home)
	require.NoError(t, err)

	data, err := os.ReadFile(got)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	mutated := append([]byte(nil), data...)
	mutated[0] ^= 0xFF // flip a byte but keep the same length
	require.NoError(t, os.WriteFile(got, mutated, 0o755))

	got2, err := selfinstall.Install(home)
	require.NoError(t, err)
	require.Equal(t, got, got2)

	after, err := os.ReadFile(got)
	require.NoError(t, err)
	require.Equal(t, mutated, after, "same-size destination must not be re-copied")
}

// TestInstall_MkdirAllFails_ReturnsError points homeDir under an existing
// plain file so MkdirAll(homeDir/bin) fails.
func TestInstall_MkdirAllFails_ReturnsError(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	home := filepath.Join(blocker, "home") // "home" can't exist: its parent is a file
	_, err := selfinstall.Install(home)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mkdir")
}

// TestInstall_CopyFileErrorPropagates_ReadOnlyBinDir pre-creates the bin dir
// read-only so copyFile's create step fails, and Install must surface that
// error rather than swallow it.
func TestInstall_CopyFileErrorPropagates_ReadOnlyBinDir(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o555)) // r-x: MkdirAll no-ops on an existing dir
	t.Cleanup(func() { _ = os.Chmod(binDir, 0o755) })

	_, err := selfinstall.Install(home)
	require.Error(t, err)
}
