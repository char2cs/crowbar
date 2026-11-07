package home

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSocket_DefaultHomeBindsInsideTheHome(t *testing.T) {
	shortTemp(t)
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
	shortTemp(t)
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

// One spelling of a home gets one socket: a trailing slash or a dot segment
// must not name a different daemon.
func TestSocket_OverrideHashesTheCleanedHome(t *testing.T) {
	base := t.TempDir()
	for _, given := range []string{base + "/", base + "//", base + "/./", base + "/x/.."} {
		t.Setenv(EnvVar, given)

		h, err := Resolve()
		require.NoError(t, err)

		assert.Equal(t, filepath.Join(os.TempDir(), socketName(base)), h.Socket(), given)
		assert.Equal(t, base, h.Root(), given)
	}
}

func TestSocketName_IsStable(t *testing.T) {
	assert.Equal(t, "crowbar-c13f09536446a88e.sock", socketName("/dev/crowbar-home"))
	assert.Equal(t, "crowbar-a4c579b364108993.sock", socketName("/dev/crowbar-home/"))
}

// Every in-PTY callback bakes a concrete --home into its command line, even on
// the plain default home. Hashing that spelled-out default sent each hook to a
// temp-dir socket the real daemon never listens on.
func TestRegression_SocketPath_HomeEnvSpellingOutThePlainDefaultIsNotAnOverride(t *testing.T) {
	shortTemp(t)
	if runtime.GOOS == "windows" {
		t.Skip("HOME does not set the user home on windows")
	}
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	plainDefault := filepath.Join(fakeHome, ".crowbar")

	t.Setenv(EnvVar, "")
	noOverride, err := Resolve()
	require.NoError(t, err)

	t.Setenv(EnvVar, plainDefault)
	spelledOut, err := Resolve()
	require.NoError(t, err)

	assert.Equal(t, noOverride.Socket(), spelledOut.Socket(),
		"a --home that only spells out the plain default must bind the daemon's own <home>/crowbar.sock")
	assert.NotEqual(t, filepath.Join(os.TempDir(), socketName(plainDefault)), spelledOut.Socket(),
		"must not fall into the dev-isolation hash path just because CROWBAR_HOME is set")
}

// The other side of the fix above: a home that differs from the plain default
// must keep its own hashed, isolated socket.
func TestRegression_SocketPath_ARealOverrideStillIsolates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	realOverride := filepath.Join(t.TempDir(), "some-worktree", ".crowbar")
	t.Setenv(EnvVar, realOverride)

	h, err := Resolve()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(os.TempDir(), socketName(realOverride)), h.Socket())
}

// A home deep enough that its socket would not fit in sun_path binds a short
// name in the user's runtime dir instead, the same for every client.
func TestSocket_APathTooLongToBindFallsBackToTheRuntimeDir(t *testing.T) {
	shortTemp(t)
	userHome := filepath.Join(t.TempDir(), strings.Repeat("deep/", 24))
	t.Setenv("HOME", userHome)
	t.Setenv(EnvVar, "")
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	h, err := Resolve()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(runtimeDir, socketName(h.Root())), h.Socket())
}

// Without a usable runtime dir the short socket lives in a per-user directory
// under /tmp, whatever TMPDIR says.
func TestSocket_WithoutARuntimeDirTheFallbackIsPerUserUnderTmp(t *testing.T) {
	root := t.TempDir()
	h, err := At(root)
	require.NoError(t, err)
	t.Setenv("TMPDIR", filepath.Join(root, strings.Repeat("long-tmp/", 12)))
	t.Setenv("XDG_RUNTIME_DIR", "")

	want := filepath.Join("/tmp", "crowbar-"+strconv.Itoa(os.Getuid()), socketName(root))
	assert.Equal(t, want, h.Socket())
	assert.LessOrEqual(t, len(h.Socket()), maxSocketPath())
}

// shortTemp points the temp dir at /tmp, so a home made under it has a socket
// path that fits in sun_path whatever TMPDIR the test run was given.
func shortTemp(t *testing.T) {
	t.Helper()
	t.Setenv("TMPDIR", "/tmp")
}
