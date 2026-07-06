package transports

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSocketPath_ExplicitPathPassthrough(t *testing.T) {
	got, err := SocketPath("unix:///tmp/explicit.sock")
	require.NoError(t, err)
	require.Equal(t, "/tmp/explicit.sock", got)
}

func TestSocketPath_CrowbarHomeHashed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CROWBAR_HOME", home)
	got, err := SocketPath("unix://")
	require.NoError(t, err)

	h := fnv.New64a()
	_, _ = h.Write([]byte(home))
	want := filepath.Join(os.TempDir(), "crowbar-"+hex64(h.Sum64())+".sock")
	require.Equal(t, want, got)
}

func hex64(v uint64) string { return fmt.Sprintf("%x", v) }
