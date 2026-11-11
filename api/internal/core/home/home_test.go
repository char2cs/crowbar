package home

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_RootsEverythingAtCrowbarHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvVar, root)

	h, err := Resolve()
	require.NoError(t, err)

	assert.Equal(t, root, h.Root())
	assert.Equal(t, filepath.Join(root, "state"), h.State())
	assert.Equal(t, filepath.Join(root, "state", "events"), h.Events())
	assert.Equal(t, filepath.Join(root, "state", "store"), h.Store())
	assert.Equal(t, filepath.Join(root, "state", "content"), h.Content())
	assert.Equal(t, filepath.Join(root, "logs", "daemon.log"), h.DaemonLog())
	assert.Equal(t, filepath.Join(root, "config.yaml"), h.Config())
	assert.Equal(t, filepath.Join(root, "LAYOUT"), h.LayoutFile())
	assert.Equal(t, filepath.Join(root, "state", ".lock"), h.LockFile())
	assert.Equal(t, filepath.Join(root, "migrate"), h.MigrateDir())
}

func TestResolve_RejectsARelativeCrowbarHome(t *testing.T) {
	t.Setenv(EnvVar, "rel/dir")

	_, err := Resolve()

	require.ErrorIs(t, err, errRelative)
}

func TestResolve_DefaultsToDotCrowbarUnderTheUserHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME does not set the user home on windows")
	}
	userHome := t.TempDir()
	t.Setenv(EnvVar, "")
	t.Setenv("HOME", userHome)

	h, err := Resolve()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(userHome, ".crowbar"), h.Root())
}

func TestResolve_RefusesAHomeOutsideTempInATestBinary(t *testing.T) {
	t.Setenv(EnvVar, realHomeDirOrSkip(t))

	_, err := Resolve()

	require.ErrorIs(t, err, errOutsideTemp)
}

func TestAt_RefusesAHomeOutsideTempInATestBinary(t *testing.T) {
	_, err := At(filepath.Join(realHomeDirOrSkip(t), ".crowbar"))

	require.ErrorIs(t, err, errOutsideTemp)
}

func TestAt_AcceptsAHomeUnderTemp(t *testing.T) {
	root := filepath.Join(t.TempDir(), "home")

	h, err := At(root)
	require.NoError(t, err)

	assert.Equal(t, root, h.Root())
}

func TestAt_AcceptsASiblingNamedLikeAParentUnderTemp(t *testing.T) {
	root := filepath.Join(os.TempDir(), "..x", "home")

	h, err := At(root)
	require.NoError(t, err)

	assert.Equal(t, root, h.Root())
}

func TestAt_RefusesTheTempDirsParent(t *testing.T) {
	_, err := At(filepath.Dir(filepath.Clean(os.TempDir())))

	require.ErrorIs(t, err, errOutsideTemp)
}

func TestAt_RejectsARelativeRoot(t *testing.T) {
	_, err := At("rel/dir")

	require.ErrorIs(t, err, errRelative)
}

func TestAt_AndResolve_NeverCreateTheHome(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "not", "yet")
	t.Setenv(EnvVar, root)

	_, atErr := At(root)
	_, resolveErr := Resolve()
	require.NoError(t, atErr)
	require.NoError(t, resolveErr)

	assert.NoDirExists(t, filepath.Join(base, "not"))
}

// A caller handed an optional directory gets that home, or the one Resolve
// names when it was handed none.
func TestResolveDir_IsAtForADirAndResolveWithout(t *testing.T) {
	override := filepath.Join(t.TempDir(), "override")
	given := filepath.Join(t.TempDir(), "given")
	t.Setenv(EnvVar, override)

	h, err := ResolveDir(given)
	require.NoError(t, err)
	assert.Equal(t, given, h.Root())

	h, err = ResolveDir("")
	require.NoError(t, err)
	assert.Equal(t, override, h.Root())

	_, err = ResolveDir("relative")
	require.ErrorIs(t, err, errRelative)
}

func TestIsDefault_IsTrueOnlyForTheUsersOwnHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME does not set the user home on windows")
	}
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	cases := map[string]bool{
		"":                                  true,
		filepath.Join(userHome, ".crowbar"): true,
		filepath.Join(userHome, ".crowbar") + "/": true,
		filepath.Join(userHome, "elsewhere"):      false,
	}
	for override, want := range cases {
		t.Setenv(EnvVar, override)

		h, err := Resolve()
		require.NoError(t, err)

		assert.Equal(t, want, h.IsDefault(), "CROWBAR_HOME=%q", override)
	}
}

func realHomeDirOrSkip(t *testing.T) string {
	t.Helper()
	current, err := user.Current()
	if err != nil || current.HomeDir == "" {
		t.Skip("no account home directory to pin the guard against")
	}
	if underDir(os.TempDir(), current.HomeDir) {
		t.Skip("the account home is under os.TempDir()")
	}
	return current.HomeDir
}
