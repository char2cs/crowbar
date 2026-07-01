package safepath_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

// TestResolve_AllowsInWorkspacePaths verifies that legitimate
// workspace-relative paths resolve to the joined absolute path.
func TestResolve_AllowsInWorkspacePaths(
	t *testing.T,
) {
	root := t.TempDir()
	cases := []struct {
		name string
		rel  string
	}{
		{"root itself", ""},
		{"top-level file", "file.txt"},
		{"nested file", "a/b/c.txt"},
		{"dot-prefixed (hidden) file", ".gitignore"},
		{"single dot", "."},
		{"dot path", "./a/b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := safepath.Resolve(root, tc.rel)
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(root, tc.rel), got)
		})
	}
}

// TestResolve_RejectsPathEscape covers every lexical traversal class:
// a "../" escape, an absolute path, and a sneaky mid-path escape.
func TestResolve_RejectsPathEscape(
	t *testing.T,
) {
	root := t.TempDir()
	cases := []struct {
		name string
		rel  string
	}{
		{"parent traversal", "../../etc/passwd"},
		{"single parent", ".."},
		{"absolute path", "/etc/passwd"},
		{"mid-path escape", "a/../../b"},
		{"trailing escape", "a/b/../../../c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := safepath.Resolve(root, tc.rel)
			require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
		})
	}
}

// TestResolve_RejectsSymlinkEscape verifies the symlink-escape defense: a
// relative path whose parent directory is a symlink pointing OUTSIDE the
// workspace root is rejected even though the path is lexically local.
func TestResolve_RejectsSymlinkEscape(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	outside := t.TempDir()
	root := t.TempDir()

	// root/link -> outside (a symlinked directory escaping the workspace).
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(outside, link))

	// Lexically local ("link/secret.txt") but resolves outside the root.
	_, err := safepath.Resolve(root, "link/secret.txt")
	require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
}

// TestResolve_RejectsSymlinkTargetEscape verifies that an existing file that
// is itself a symlink to a target outside the root is rejected.
func TestResolve_RejectsSymlinkTargetEscape(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o600))
	root := t.TempDir()

	// root/escape.txt -> outside/secret.txt
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "escape.txt")))

	_, err := safepath.Resolve(root, "escape.txt")
	require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
}

// TestResolve_AllowsSymlinkInsideRoot verifies an internal symlink (target
// inside the workspace) is still permitted.
func TestResolve_AllowsSymlinkInsideRoot(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "real"), 0o700))
	require.NoError(t, os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "alias")))

	got, err := safepath.Resolve(root, "alias/file.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "alias/file.txt"), got)
}

// TestResolve_AllowsNewFileUnderHonestParent verifies the new-file edge: a
// not-yet-existing target whose existing ancestors are all inside the root is
// allowed (so create/write of new files keeps working).
func TestResolve_AllowsNewFileUnderHonestParent(
	t *testing.T,
) {
	root := t.TempDir()
	got, err := safepath.Resolve(root, "does/not/exist/yet.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "does/not/exist/yet.txt"), got)
}
