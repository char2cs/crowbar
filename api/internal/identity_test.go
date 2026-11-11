package internal_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// writeFileInPlaceOfDir leaves path unusable as a directory, whoever runs the
// test: unlike a permission bit, root cannot write through it.
func writeFileInPlaceOfDir(
	t *testing.T,
	path string,
) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, nil, 0o600))
}

func writeCorruptIdentity(
	t *testing.T,
	tlsDir string,
) {
	t.Helper()
	require.NoError(t, os.MkdirAll(tlsDir, 0o700))
	for _, name := range []string{"cert.pem", "key.pem"} {
		require.NoError(t, os.WriteFile(filepath.Join(tlsDir, name), []byte("not a pem"), 0o600))
	}
}

// TestRegression_New_LocalDaemonNeverTouchesTheTLSDir proves a daemon serving
// no TLS — the unix socket alone, or the loopback dev bind — boots whatever
// state its TLS identity directory is in, and never creates one.
func TestRegression_New_LocalDaemonNeverTouchesTheTLSDir(t *testing.T) {
	spoilers := map[string]func(t *testing.T, tlsDir string){
		"absent":    func(*testing.T, string) {},
		"not a dir": writeFileInPlaceOfDir,
		"corrupt":   writeCorruptIdentity,
	}
	for _, host := range []string{"unix", "tcp://127.0.0.1:0"} {
		for name, spoil := range spoilers {
			t.Run(host+" "+name, func(t *testing.T) {
				h := testutil.FixtureHome(t)
				tlsDir := filepath.Join(h.Root(), "tls")
				spoil(t, tlsDir)
				bind := host
				if host == "unix" {
					bind = "unix://" + testutil.ShortSocket(t)
				}

				c, err := internal.New(context.Background(), bind, nil, internal.WithHomeDir(h.Root()))
				require.NoError(t, err)
				t.Cleanup(c.Close)

				if name == "absent" {
					_, statErr := os.Stat(tlsDir)
					assert.ErrorIs(t, statErr, fs.ErrNotExist, "a local daemon must not create a TLS identity")
				}
			})
		}
	}
}

// TestRegression_New_ExposedBindRefusesAnUnloadableIdentity proves a corrupt
// identity is never silently replaced on a network-exposed daemon — every
// paired client pins it — and that the refusal says what replacing it means.
func TestRegression_New_ExposedBindRefusesAnUnloadableIdentity(t *testing.T) {
	h := testutil.FixtureHome(t)
	tlsDir := filepath.Join(h.Root(), "tls")
	writeCorruptIdentity(t, tlsDir)

	_, err := internal.New(context.Background(), "tcp://localhost.:0", nil, internal.WithHomeDir(h.Root()))

	require.ErrorContains(t, err, "identity cannot be loaded")
	require.ErrorContains(t, err, "identity change")
	cert, readErr := os.ReadFile(filepath.Join(tlsDir, "cert.pem"))
	require.NoError(t, readErr)
	assert.Equal(t, "not a pem", string(cert), "the corrupt identity must be left for the operator, not regenerated")
}
