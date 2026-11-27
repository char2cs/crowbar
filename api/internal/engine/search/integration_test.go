//go:build integration

package search_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/search"
)

func TestIntegration_BasicSearch(
	t *testing.T,
) {
	dir := t.TempDir()
	eng := search.New()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main\n\n// hello world\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor", "lib.go"), []byte("// hello vendor\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("vendor/\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "binary.bin"), []byte{0x00, 0x01, 0x68, 0x65, 0x6c, 0x6c, 0x6f}, 0o600))

	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
	})
	require.NoError(t, err)

	found := false
	for _, r := range resp.Results {
		if r.FilePath == "src/main.go" {
			found = true
		}
		assert.NotContains(t, r.FilePath, "vendor/")
		assert.NotEqual(t, "binary.bin", r.FilePath)
	}
	assert.True(t, found, "src/main.go should be in results")
}

func TestIntegration_WholeWord(
	t *testing.T,
) {
	dir := t.TempDir()
	eng := search.New()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.go"), []byte("hello world\nhellofoo\n"), 0o600))

	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
		WholeWord:     true,
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, 1, resp.Results[0].LineNumber)

	resp2, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hell",
		CaseSensitive: true,
		WholeWord:     true,
	})
	require.NoError(t, err)
	assert.Empty(t, resp2.Results)
}

func TestIntegration_ResultCap(
	t *testing.T,
) {
	dir := t.TempDir()
	eng := search.New()

	for i := 0; i < 1500; i++ {
		name := filepath.Join(dir, fmt.Sprintf("file%04d.txt", i))
		require.NoError(t, os.WriteFile(name, []byte("match\n"), 0o600))
	}

	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "match",
		CaseSensitive: true,
	})
	require.NoError(t, err)
	assert.True(t, resp.Truncated)
	assert.Equal(t, 1000, len(resp.Results))
}

func TestIntegration_SubdirGitignoreNegation(
	t *testing.T,
) {
	dir := t.TempDir()
	eng := search.New()

	subDir := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, ".gitignore"), []byte("!important.log\n"), 0o600))

	require.NoError(t, os.WriteFile(filepath.Join(subDir, "important.log"), []byte("hello important\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "debug.log"), []byte("hello debug\n"), 0o600))

	resp, err := eng.Search(context.Background(), dir, search.SearchRequest{
		Query:         "hello",
		CaseSensitive: true,
	})
	require.NoError(t, err)

	foundImportant := false
	for _, r := range resp.Results {
		assert.NotEqual(t, "sub/debug.log", r.FilePath, "debug.log should be excluded")
		if r.FilePath == "sub/important.log" {
			foundImportant = true
		}
	}
	assert.True(t, foundImportant, "sub/important.log should appear in results")
}

func TestIntegration_Replace(
	t *testing.T,
) {
	dir := t.TempDir()
	eng := search.New()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	filePath := filepath.Join(dir, "src", "main.go")
	require.NoError(t, os.WriteFile(filePath, []byte("hello world\n"), 0o600))

	err := eng.Replace(context.Background(), dir, search.ReplaceRequest{
		Query:         "hello",
		Replacement:   "goodbye",
		Scope:         "file:src/main.go",
		CaseSensitive: true,
	}, false)
	require.NoError(t, err)

	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, "goodbye world\n", string(data))
}
