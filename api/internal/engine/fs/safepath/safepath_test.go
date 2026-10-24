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

// TestCanonical_ResolvesSymlinkedParentOfNewFile verifies a file that does not
// exist yet gets the same spelling through a symlinked parent as through its
// real directory, so one file has one identity before and after it is created.
func TestCanonical_ResolvesSymlinkedParentOfNewFile(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(dir, link))

	want := filepath.Join(safepath.Canonical(dir), "new", "file.md")
	assert.Equal(t, want, safepath.Canonical(filepath.Join(link, "new", "file.md")))
	assert.Equal(t, want, safepath.Canonical(filepath.Join(dir, "new", "..", "new", "file.md")))
}

// TestLocate_MatchesByFileIdentity verifies the walk compares directories, not
// spellings: a path through a symlinked alias of a root is under that root, the
// nearest root wins, and every root that is that directory is reported.
func TestLocate_MatchesByFileIdentity(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	parent := t.TempDir()
	wt := filepath.Join(parent, "wt")
	require.NoError(t, os.Mkdir(wt, 0o700))
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(wt, alias))
	roots := []string{parent, "", filepath.Join(parent, "gone"), wt, wt}

	hits, rel := safepath.Locate(filepath.Join(alias, "src", "a.md"), roots)
	assert.Equal(t, []int{3, 4}, hits)
	assert.Equal(t, filepath.Join("src", "a.md"), rel)

	hits, _ = safepath.Locate(filepath.Join(t.TempDir(), "a.md"), roots)
	assert.Empty(t, hits, "a path under no root is located nowhere")
}

// TestLocate_SpellsTheRestAsOnDisk verifies one file has one spelling on a
// case-insensitive volume, whatever case the caller typed.
func TestLocate_SpellsTheRestAsOnDisk(
	t *testing.T,
) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "a.md"), nil, 0o600))
	shouted := filepath.Join(root, "SRC", "A.md")
	if _, err := os.Stat(shouted); err != nil {
		t.Skip("case-sensitive volume")
	}

	_, rel := safepath.Locate(shouted, []string{root})
	assert.Equal(t, filepath.Join("src", "a.md"), rel)
}

// TestLocate_SpellsNormalizationAsOnDisk verifies one file has one spelling on
// a normalization-insensitive volume: a name typed in NFC that reaches an NFD
// entry, or the reverse, resolves to the entry's own spelling.
func TestLocate_SpellsNormalizationAsOnDisk(
	t *testing.T,
) {
	nfc, nfd := "café.md", "café.md"
	for _, pair := range [][2]string{{nfd, nfc}, {nfc, nfd}} {
		onDisk, typed := pair[0], pair[1]
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, onDisk), nil, 0o600))
		if _, err := os.Stat(filepath.Join(root, typed)); err != nil {
			t.Skip("normalization-sensitive volume")
		}

		_, rel := safepath.Locate(filepath.Join(root, typed), []string{root})
		assert.Equal(t, onDisk, rel)
	}
}
