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
