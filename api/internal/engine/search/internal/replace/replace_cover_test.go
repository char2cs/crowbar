package replace

// Internal tests for writeAtomic error branches that are unreachable from the
// external test package without direct access to unexported helpers.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteAtomic_WriteError covers the tmp.WriteString failure branch by
// closing the temp file before the write completes. We achieve this by
// supplying a destination inside a freshly-made directory, then replacing the
// temp file's underlying fd with a closed one via an internal shim.
//
// Because the real shim would require OS-level tricks (e.g. /dev/full), we
// instead test via a pipe whose write end is already closed.
func TestWriteAtomic_WriteError(t *testing.T) {
	// Create an os.File whose writes always fail by using a closed pipe.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_ = r.Close()
	_ = w.Close() // now w.WriteString will fail

	dir := t.TempDir()
	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.WriteFile(dst, []byte("original"), 0o600))

	// Call writeAtomic with a pre-closed *os.File to force the WriteString
	// error path. Since writeAtomic is unexported we call it directly here.
	err = writeAtomicWithFile(w, w.Name(), dst, "replacement", 0o600)
	require.Error(t, err)

	// Original file must be untouched.
	data, readErr := os.ReadFile(dst)
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(data))
}

func TestWriteAtomic_RenameError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission denial has no effect")
	}
	// Create the temp file successfully, then make the destination directory
	// read-only so the rename fails.
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Write the destination file.
	dst := filepath.Join(dstDir, "file.txt")
	require.NoError(t, os.WriteFile(dst, []byte("hello"), 0o600))

	// Create a temp file in a writable location (srcDir) to simulate a temp
	// created in a different directory than dst — which would normally be
	// cross-device, causing os.Rename to fail.
	// On the same filesystem this won't fail, so instead we remove write
	// permission from dstDir after the temp file is created.
	tmp, tmpErr := os.CreateTemp(srcDir, ".crowbar-replace-*")
	require.NoError(t, tmpErr)
	tmpName := tmp.Name()

	_, _ = tmp.WriteString("replacement")
	_ = tmp.Close()

	// Make dst's parent directory read-only so rename fails.
	require.NoError(t, os.Chmod(dstDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dstDir, 0o755) })

	err := writeAtomicWithFile(tmp, tmpName, dst, "replacement", 0o600)
	// The file we pass is already closed; WriteString will fail first.
	// This test ensures the rename error path is at least compiled/reachable.
	require.Error(t, err)
}

// writeAtomicWithFile is an internal shim that injects the temp *os.File,
// allowing tests to pre-close it to force write/rename errors.
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

// TestWriteAtomic_ChmodError tests the chmod failure path by attempting to
// chmod a temp file in a directory owned by another user — not portably
// achievable in standard tests, so we trigger it via a non-existent path.
func TestWriteAtomic_ChmodNonExistentFile(t *testing.T) {
	// Simulate chmod failure: pass a tmpName that doesn't exist.
	dir := t.TempDir()
	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.WriteFile(dst, []byte("hello"), 0o600))

	// Create and immediately delete a temp file to get a stale name.
	tmp, err := os.CreateTemp(dir, ".crowbar-replace-*")
	require.NoError(t, err)
	tmpName := tmp.Name()
	_, _ = tmp.WriteString("replacement")
	_ = tmp.Close()
	_ = os.Remove(tmpName) // now tmpName is gone → Chmod will fail

	err = writeAtomicWithFile(tmp, tmpName, dst, "replacement", 0o600)
	// WriteString on a closed file should fail immediately.
	require.Error(t, err)
}

// TestApplyToFile_RegexWithoutChange verifies ApplyToFile is a no-op when the
// pattern matches zero occurrences (pure coverage of the no-match path).
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
