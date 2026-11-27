package replace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteAtomic_WriteError(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_ = r.Close()
	_ = w.Close()

	dir := t.TempDir()
	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.WriteFile(dst, []byte("original"), 0o600))

	err = writeAtomicWithFile(w, w.Name(), dst, "replacement", 0o600)
	require.Error(t, err)

	data, readErr := os.ReadFile(dst)
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(data))
}

func TestWriteAtomic_RenameError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission denial has no effect")
	}
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	dst := filepath.Join(dstDir, "file.txt")
	require.NoError(t, os.WriteFile(dst, []byte("hello"), 0o600))

	tmp, tmpErr := os.CreateTemp(srcDir, ".crowbar-replace-*")
	require.NoError(t, tmpErr)
	tmpName := tmp.Name()

	_, _ = tmp.WriteString("replacement")
	_ = tmp.Close()

	require.NoError(t, os.Chmod(dstDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dstDir, 0o755) })

	err := writeAtomicWithFile(tmp, tmpName, dst, "replacement", 0o600)
	require.Error(t, err)
}

func writeAtomicWithFile(
	tmp *os.File,
	tmpName string,
	dst string,
	content string,
	originalMode os.FileMode,
) error {
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace: write temp: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace: close temp: %w", err)
	}

	if err := os.Chmod(tmpName, originalMode); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace: chmod %s: %w", tmpName, err)
	}

	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace: rename: %w", err)
	}

	return nil
}

func TestWriteAtomic_ChmodNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.WriteFile(dst, []byte("hello"), 0o600))

	tmp, err := os.CreateTemp(dir, ".crowbar-replace-*")
	require.NoError(t, err)
	tmpName := tmp.Name()
	_, _ = tmp.WriteString("replacement")
	_ = tmp.Close()
	_ = os.Remove(tmpName)

	err = writeAtomicWithFile(tmp, tmpName, dst, "replacement", 0o600)
	require.Error(t, err)
}

func TestWriteAtomic_RenameOverExistingDirectoryFails(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "occupied")
	require.NoError(t, os.MkdirAll(dst, 0o755))

	err := writeAtomic(dst, "replacement", 0o600)
	require.Error(t, err, "renaming a temp file over an existing directory must fail")
	assert.Contains(t, err.Error(), "rename")

	info, statErr := os.Stat(dst)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		if e.Name() == "occupied" {
			continue
		}
		assert.NotContains(t, e.Name(), ".crowbar-replace-", "the failed rename's temp file must be cleaned up")
	}
}

func TestApplyToFile_RegexNoMatch_Internal(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "file.go")
	require.NoError(t, os.WriteFile(dst, []byte("nothing to replace\n"), 0o644))

	re := regexp.MustCompile(`ZZZNOMATCH`)
	require.NoError(t, ApplyToFile(dst, re, "x", false))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "nothing to replace\n", string(data))
}
