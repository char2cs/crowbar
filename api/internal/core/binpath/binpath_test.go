package binpath

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_FindsBinaryOnPath(t *testing.T) {
	// "go" is guaranteed to be on PATH in any environment running these tests.
	got := Resolve("go")
	assert.True(t, filepath.IsAbs(got), "got %q", got)
}

func TestResolve_UnknownNameReturnsNameUnchanged(t *testing.T) {
	got := Resolve("definitely-not-a-real-binary-xyz")
	assert.Equal(t, "definitely-not-a-real-binary-xyz", got)
}

// TestWellKnownDirs_IncludesUserLocalBin pins the dir the agentic CLIs actually
// install into. claude and codex land in ~/.local/bin, which no login-shell PATH
// probe is guaranteed to surface: a login NON-interactive shell (what shellenv
// runs) sources .zprofile/.zshenv but never .zshrc, and .zshrc is where that dir
// is conventionally added. Without this entry the packaged .app cannot spawn a
// vendor CLI at all.
func TestWellKnownDirs_IncludesUserLocalBin(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	assert.Contains(t, wellKnownDirs(), filepath.Join(home, ".local", "bin"))
}

func TestResolveInternal_FallsBackToWellKnownDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable-bit probing is POSIX only")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-tool")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))

	got := resolve("fake-tool", []string{dir})
	assert.Equal(t, bin, got)
}

func TestResolveInternal_SkipsNonExecutableAndDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable-bit probing is POSIX only")
	}
	dir := t.TempDir()
	// A non-executable file must not be resolved.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plain-file-tool"), []byte("x"), 0o644))
	// A directory with the binary's name must not be resolved.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "dir-tool"), 0o755))

	assert.Equal(t, "plain-file-tool", resolve("plain-file-tool", []string{dir}))
	assert.Equal(t, "dir-tool", resolve("dir-tool", []string{dir}))
}
