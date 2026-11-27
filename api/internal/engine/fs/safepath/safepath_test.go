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

func TestResolve_RejectsSymlinkEscape(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	outside := t.TempDir()
	root := t.TempDir()

	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(outside, link))

	_, err := safepath.Resolve(root, "link/secret.txt")
	require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
}

func TestResolve_RejectsSymlinkTargetEscape(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o600))
	root := t.TempDir()

	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "escape.txt")))

	_, err := safepath.Resolve(root, "escape.txt")
	require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
}

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

func TestResolve_AllowsNewFileUnderHonestParent(
	t *testing.T,
) {
	root := t.TempDir()
	got, err := safepath.Resolve(root, "does/not/exist/yet.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "does/not/exist/yet.txt"), got)
}

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
