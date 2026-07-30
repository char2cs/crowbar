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

// resetGit clears the process-wide cache so a test starts from an unresolved
// state and leaves one behind, whatever order the tests run in.
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

// TestGit_ResolvesExactlyOnce is the load-bearing claim of the whole change: a
// resolution that ran per call — worse, one that spawned anything per call —
// would cost more than the shim it replaces. Pointer identity is the proof.
// gitPath is only ever stored to by a resolution, so an unchanged pointer over
// repeated calls means no second resolution happened.
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

// TestGit_RunsTheSameGitAsPath is the semantic guard. This change is only ever
// allowed to alter WHICH FILE is exec'd, never which git the user gets, so the
// resolved binary must report the identical version to a bare `git` off PATH.
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

// TestResolveGit_LooksPastTheXcrunShim pins the actual optimisation: when PATH
// hands back the /usr/bin stub, the developer-directory binary behind it is
// used instead.
func TestResolveGit_LooksPastTheXcrunShim(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the xcrun shim is macOS only")
	}
	dev := fakeDeveloperDir(t)

	got := resolveGit(xcrunShimPath, []string{dev})

	assert.Equal(t, filepath.Join(dev, "usr", "bin", "git"), got)
}

// TestResolveGit_FallsBackToTheShimWhenNoDeveloperDirHasGit is the fallback
// that makes this safe to ship: a machine with no Xcode and no Command Line
// Tools keeps the git it had. Resolution failing must never fail a git call.
func TestResolveGit_FallsBackToTheShimWhenNoDeveloperDirHasGit(t *testing.T) {
	got := resolveGit(xcrunShimPath, []string{"/nonexistent/developer/dir", t.TempDir()})

	assert.Equal(t, xcrunShimPath, got)
}

// TestResolveGit_LeavesANonShimPathAlone covers Linux, a stripped container,
// and a macOS user whose PATH prefers Homebrew git: whatever PATH chose is what
// runs, only as an absolute path.
func TestResolveGit_LeavesANonShimPathAlone(t *testing.T) {
	dev := fakeDeveloperDir(t)

	got := resolveGit("/opt/homebrew/bin/git", []string{dev})

	assert.Equal(t, "/opt/homebrew/bin/git", got)
}

// TestResolveGit_UnresolvableGitStaysBare keeps the pre-existing failure mode
// intact: with no git anywhere, the caller still execs "git" and still gets the
// usual "executable file not found" rather than something new to interpret.
func TestResolveGit_UnresolvableGitStaysBare(t *testing.T) {
	got := resolveGit("git", []string{"/nonexistent/developer/dir"})

	assert.Equal(t, "git", got)
}

func TestDeveloperDirs_HonoursTheDeveloperDirOverride(t *testing.T) {
	t.Setenv("DEVELOPER_DIR", "/custom/developer/dir")

	assert.Equal(t, "/custom/developer/dir", developerDirs()[0])
}

func TestDeveloperDirs_IncludesTheStockInstallLocations(t *testing.T) {
	t.Setenv("DEVELOPER_DIR", "")
	dirs := developerDirs()

	assert.Contains(t, dirs, "/Library/Developer/CommandLineTools")
	assert.Contains(t, dirs, "/Applications/Xcode.app/Contents/Developer")
}

// TestRecoverGit_ReResolvesAVanishedBinary covers an Xcode or Command Line
// Tools update replacing the developer directory under a running daemon. The
// cached path names a file that no longer exists; without recovery every git
// call for the rest of the process's life would fail to exec.
func TestRecoverGit_ReResolvesAVanishedBinary(t *testing.T) {
	resetGit(t)
	vanished := filepath.Join(t.TempDir(), "usr", "bin", "git")
	gitPath.Store(&vanished)

	assert.True(t, RecoverGit(), "a vanished binary must be re-resolved")
	assert.NotEqual(t, vanished, Git())
	assert.True(t, isExecutableFile(Git()) || Git() == gitName, "got %q", Git())
}

// TestRecoverGit_LeavesALiveResolutionAlone is the guard on the guard. Recovery
// is driven by a stat of the cached path, not by the exec error, because a
// deleted working directory produces the same "fork/exec: no such file or
// directory" as a deleted binary — and re-resolving on that would spend an
// extra spawn on every call made against a repo the user removed.
func TestRecoverGit_LeavesALiveResolutionAlone(t *testing.T) {
	resetGit(t)
	before := Git()
	if !filepath.IsAbs(before) {
		t.Skip("no git resolved on this machine")
	}

	assert.False(t, RecoverGit(), "a live binary must not be re-resolved")
	assert.Equal(t, before, Git())
}

// BenchmarkGit is the counter-check on the optimisation. Resolution that ran
// per call would be worse than the shim it replaces — a single stat is already
// a syscall, and a single spawn would be a hundred times the cost of the ~6ms
// it saves. A steady-state Git call must be an atomic load and nothing else,
// which shows up as low single-digit nanoseconds and zero allocations.
//
//	go test -run='^$' -bench=BenchmarkGit ./internal/core/binpath/
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

// TestRecoverGit_BareNameDoesNotRecover keeps the no-git-anywhere machine off
// the recovery path. "git" never stats as an executable file, so a relative
// cached value would otherwise re-resolve on every single failed call.
func TestRecoverGit_BareNameDoesNotRecover(t *testing.T) {
	resetGit(t)
	bare := gitName
	gitPath.Store(&bare)

	assert.False(t, RecoverGit())
	assert.Equal(t, gitName, Git())
}
