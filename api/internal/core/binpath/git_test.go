package binpath

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetGit(
	t *testing.T,
) {
	t.Helper()
	gitPath.Store(nil)
	t.Cleanup(func() { gitPath.Store(nil) })
}

func fakeDeveloperDir(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "usr", "bin")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\n"), 0o755))
	return dir
}

func TestGit_ResolvesExactlyOnce(t *testing.T) {
	resetGit(t)
	require.Nil(t, gitPath.Load(), "cache must start empty")

	first := Git()
	stored := gitPath.Load()
	require.NotNil(t, stored, "the first call must populate the cache")

	for range 10 {
		assert.Equal(t, first, Git())
		assert.Same(t, stored, gitPath.Load(), "Git re-resolved instead of reading the cache")
	}
}

func TestGit_RunsTheSameGitAsPath(t *testing.T) {
	resetGit(t)

	want, err := exec.Command("git", "--version").Output()
	if err != nil {
		t.Skip("no git on PATH")
	}
	got, err := exec.Command(Git(), "--version").Output()
	require.NoError(t, err, "the resolved git must run")

	assert.Equal(t, strings.TrimSpace(string(want)), strings.TrimSpace(string(got)))
}

func TestResolveGit_LooksPastTheXcrunShim(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the xcrun shim is macOS only")
	}
	dev := fakeDeveloperDir(t)

	got := resolveGit(xcrunShimPath, []string{dev})

	assert.Equal(t, filepath.Join(dev, "usr", "bin", "git"), got)
}

func TestResolveGit_FallsBackToTheShimWhenNoDeveloperDirHasGit(t *testing.T) {
	dirs := []string{"/nonexistent/developer/dir", t.TempDir()}

	assert.Empty(t, developerGit(dirs), "no candidate dir contains an executable git")
	assert.Equal(t, xcrunShimPath, resolveGit(xcrunShimPath, dirs))
}

func TestDeveloperGit_SkipsADirWithoutAnExecutableGit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable-bit probing is POSIX only")
	}
	broken := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(broken, "usr", "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(broken, "usr", "bin", "git"), []byte("x"), 0o644))
	working := fakeDeveloperDir(t)

	got := developerGit([]string{broken, working})

	assert.Equal(t, filepath.Join(working, "usr", "bin", "git"), got)
}

func TestResolveGit_LeavesANonShimPathAlone(t *testing.T) {
	dev := fakeDeveloperDir(t)

	got := resolveGit("/opt/homebrew/bin/git", []string{dev})

	assert.Equal(t, "/opt/homebrew/bin/git", got)
}

func TestResolveGit_UnresolvableGitStaysBare(t *testing.T) {
	got := resolveGit("git", []string{"/nonexistent/developer/dir"})

	assert.Equal(t, "git", got)
}

func TestDeveloperDirs_HonoursTheDeveloperDirOverride(t *testing.T) {
	t.Setenv("DEVELOPER_DIR", "/custom/developer/dir")

	assert.Equal(t, "/custom/developer/dir", developerDirs()[0])
}

func TestRecoverGit_ReResolvesAVanishedBinary(t *testing.T) {
	resetGit(t)
	vanished := filepath.Join(t.TempDir(), "usr", "bin", "git")
	gitPath.Store(&vanished)

	assert.True(t, RecoverGit(), "a vanished binary must be re-resolved")
	assert.NotEqual(t, vanished, Git())
	assert.True(t, isExecutableFile(Git()) || Git() == gitName, "got %q", Git())
}

func TestRecoverGit_LeavesALiveResolutionAlone(t *testing.T) {
	resetGit(t)
	before := Git()
	if !filepath.IsAbs(before) {
		t.Skip("no git resolved on this machine")
	}

	assert.False(t, RecoverGit(), "a live binary must not be re-resolved")
	assert.Equal(t, before, Git())
}

func BenchmarkGit(
	b *testing.B,
) {
	Git()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = Git()
	}
}

func TestRecoverGit_UnresolvedCacheDoesNotRecover(t *testing.T) {
	resetGit(t)

	assert.False(t, RecoverGit())
	assert.Nil(t, gitPath.Load(), "recovery must not populate an empty cache")
}

func TestRecoverGit_BareNameDoesNotRecover(t *testing.T) {
	resetGit(t)
	bare := gitName
	gitPath.Store(&bare)

	assert.False(t, RecoverGit())
	assert.Equal(t, gitName, Git())
}
