package search_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/search"
)

func newEngine() search.SearchEngine {
	return search.New()
}

func buildRepo(
	t *testing.T,
	files map[string]string,
) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(content), 0o600))
	}
	return dir
}

func TestSearch_BasicMatch(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"src/main.go": "package main\n\nfunc main() {\n\t// hello world\n}\n",
	})

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "src/main.go", resp.Results[0].FilePath)
	assert.Equal(t, 4, resp.Results[0].LineNumber)
	assert.False(t, resp.Truncated)
}

func TestSearch_CaseInsensitive(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"file.go": "HELLO world\n",
	})

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: false,
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
}

func TestSearch_WholeWord(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"file.go": "hellofoo\nhello world\n",
	})

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
		WholeWord:     true,
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, 2, resp.Results[0].LineNumber)
}

func TestSearch_RegexMode(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"file.go": "foo123\nfoobar\n",
	})

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         `foo\d+`,
		CaseSensitive: true,
		Regex:         true,
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Contains(t, resp.Results[0].LineText, "foo123")
}

func TestSearch_BinarySkipped(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"text.go": "hello world\n",
	})
	// Write a binary file.
	binPath := filepath.Join(dir, "binary.bin")
	require.NoError(t, os.WriteFile(binPath, []byte{0x00, 0x01, 0x02}, 0o600))

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
	})
	require.NoError(t, err)
	for _, r := range resp.Results {
		assert.NotEqual(t, "binary.bin", r.FilePath)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"file.go": "hello\n",
	})

	eng := newEngine()
	_, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query: "",
	})
	require.Error(t, err)
}

func TestSearch_IncludeGlob(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"main.go": "hello\n",
		"main.ts": "hello\n",
	})

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
		Include:       []string{"*.go"},
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Contains(t, resp.Results[0].FilePath, ".go")
}

func TestSearch_ExcludeGlob(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"main.go":      "hello\n",
		"main_test.go": "hello\n",
	})

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
		Exclude:       []string{"*_test.go"},
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.NotContains(t, resp.Results[0].FilePath, "_test")
}

func TestSearch_GitignoreRespected(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		".gitignore":    "vendor/\n",
		"src/main.go":   "hello world\n",
		"vendor/lib.go": "hello vendor\n",
	})

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
	})
	require.NoError(t, err)

	for _, r := range resp.Results {
		assert.NotContains(t, r.FilePath, "vendor/")
	}

	found := false
	for _, r := range resp.Results {
		if r.FilePath == "src/main.go" {
			found = true
		}
	}
	assert.True(t, found, "src/main.go should appear in results")
}

func TestReplace_FileScope(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"src/main.go": "hello world\nhello again\n",
		"other.go":    "hello other\n",
	})

	eng := newEngine()
	err := eng.Replace(context.Background(), dir, search.ReplaceRequest{
		Query:         "hello",
		Replacement:   "goodbye",
		Scope:         "file:src/main.go",
		CaseSensitive: true,
	}, false)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "src/main.go"))
	require.NoError(t, err)
	assert.Equal(t, "goodbye world\ngoodbye again\n", string(data))

	// other.go must be untouched.
	data, err = os.ReadFile(filepath.Join(dir, "other.go"))
	require.NoError(t, err)
	assert.Equal(t, "hello other\n", string(data))
}

func TestReplace_AllScope(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"a.go": "hello\n",
		"b.go": "hello\n",
	})

	eng := newEngine()
	err := eng.Replace(context.Background(), dir, search.ReplaceRequest{
		Query:         "hello",
		Replacement:   "bye",
		Scope:         "all",
		CaseSensitive: true,
	}, false)
	require.NoError(t, err)

	for _, name := range []string{"a.go", "b.go"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.Equal(t, "bye\n", string(data))
	}
}

func TestReplace_Locked(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"file.go": "hello\n",
	})

	eng := newEngine()
	err := eng.Replace(context.Background(), dir, search.ReplaceRequest{
		Query:       "hello",
		Replacement: "bye",
		Scope:       "file:file.go",
	}, true)
	require.ErrorIs(t, err, search.ErrLocked)
}

func TestReplace_InvalidPattern(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"file.go": "hello\n",
	})

	eng := newEngine()
	err := eng.Replace(context.Background(), dir, search.ReplaceRequest{
		Query:       "[invalid",
		Replacement: "x",
		Scope:       "all",
		Regex:       true,
	}, false)
	require.Error(t, err)
}

func TestSearch_TruncationAtExactBoundary(t *testing.T) {
	// Create 1001 files each containing one "needle" match to force truncation.
	dir := t.TempDir()
	for i := 0; i < 1001; i++ {
		name := filepath.Join(dir, fmt.Sprintf("file%04d.txt", i))
		require.NoError(t, os.WriteFile(name, []byte("needle\n"), 0o600))
	}

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "needle",
		CaseSensitive: true,
	})
	require.NoError(t, err)
	assert.True(t, resp.Truncated)
	assert.Equal(t, 1000, len(resp.Results))
}

func TestSearch_TruncationMidFile(t *testing.T) {
	// One file with 1001 matches triggers mid-file truncation.
	dir := t.TempDir()
	var lines []string
	for i := 0; i < 1001; i++ {
		lines = append(lines, "needle")
	}
	content := strings.Join(lines, "\n") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.txt"), []byte(content), 0o600))

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "needle",
		CaseSensitive: true,
	})
	require.NoError(t, err)
	assert.True(t, resp.Truncated)
	assert.Equal(t, 1000, len(resp.Results))
}

func TestSearch_MultipleMatchesPerLine(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"file.go": "foo foo foo\n",
	})

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "foo",
		CaseSensitive: true,
	})
	require.NoError(t, err)
	assert.Len(t, resp.Results, 3)
}

func TestReplace_FileNotFound(t *testing.T) {
	// Scope points to a non-existent file — replace.ApplyToFile returns a non-locked error.
	dir := t.TempDir()

	eng := newEngine()
	err := eng.Replace(context.Background(), dir, search.ReplaceRequest{
		Query:         "hello",
		Replacement:   "bye",
		Scope:         "file:nonexistent.go",
		CaseSensitive: true,
	}, false)
	require.Error(t, err)
	assert.NotErrorIs(t, err, search.ErrLocked)
}

func TestReplace_PathTraversal(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"file.go": "hello\n",
	})

	eng := newEngine()
	err := eng.Replace(context.Background(), dir, search.ReplaceRequest{
		Query:         "hello",
		Replacement:   "bye",
		Scope:         "file:../../etc/passwd",
		CaseSensitive: true,
	}, false)
	require.Error(t, err)
	require.ErrorIs(t, err, search.ErrPathOutsideWorkspace)
}

func TestReplace_BadPatternError(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"file.go": "hello\n",
	})

	eng := newEngine()
	err := eng.Replace(context.Background(), dir, search.ReplaceRequest{
		Query:       "[invalid",
		Replacement: "x",
		Scope:       "all",
		Regex:       true,
	}, false)
	require.Error(t, err)
	require.ErrorIs(t, err, search.ErrBadPattern)
}

func TestSearch_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 100; i++ {
		name := filepath.Join(dir, fmt.Sprintf("file%d.txt", i))
		require.NoError(t, os.WriteFile(name, []byte("hello\n"), 0o600))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	eng := newEngine()
	// Should return without hanging.
	_, err := eng.Search(ctx, dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
	})
	// May or may not error; just must not hang.
	_ = err
}

func TestSearch_NonExistentRepo(t *testing.T) {
	eng := newEngine()
	_, err := eng.Search(context.Background(), "/nonexistent/path/that/does/not/exist", search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
	})
	require.Error(t, err)
}

func TestReplace_AllScope_NonExistentRepo(t *testing.T) {
	eng := newEngine()
	err := eng.Replace(context.Background(), "/nonexistent/path/that/does/not/exist", search.ReplaceRequest{
		Query:         "hello",
		Replacement:   "bye",
		Scope:         "all",
		CaseSensitive: true,
	}, false)
	require.Error(t, err)
}

func TestSearch_GitDirIgnored(t *testing.T) {
	dir := buildRepo(t, map[string]string{
		"src/main.go": "hello world\n",
	})
	// Create a .git directory with a file containing the search term.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("hello in git\n"), 0o600))

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
	})
	require.NoError(t, err)

	// .git dir must be excluded.
	for _, r := range resp.Results {
		assert.NotContains(t, r.FilePath, ".git")
	}
	// src/main.go must appear.
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "src/main.go", resp.Results[0].FilePath)
}

func TestSearch_UnreadableFileSkipped(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission denial has no effect")
	}
	dir := buildRepo(t, map[string]string{
		"readable.go":   "hello world\n",
		"unreadable.go": "hello secret\n",
	})

	require.NoError(t, os.Chmod(filepath.Join(dir, "unreadable.go"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "unreadable.go"), 0o600) })

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Contains(t, resp.Results[0].FilePath, "readable")
}

func TestSearch_UnreadableDirSkipped(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission denial has no effect")
	}
	dir := buildRepo(t, map[string]string{
		"readable.go": "hello world\n",
		"subdir/a.go": "hello sub\n",
	})

	require.NoError(t, os.Chmod(filepath.Join(dir, "subdir"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "subdir"), 0o755) })

	eng := newEngine()
	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
	})
	require.NoError(t, err)
	// Only the readable file should appear.
	require.Len(t, resp.Results, 1)
	assert.Contains(t, resp.Results[0].FilePath, "readable")
}
