package content_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/content"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

func TestRead_TextFile(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0o600))

	fc, err := content.Read(dir, "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello world", fc.Content)
	assert.Empty(t, fc.Encoding)
}

func TestRead_BinaryFile(
	t *testing.T,
) {
	dir := t.TempDir()
	data := []byte{0x00, 0x01, 0x02, 0x03, 0xFF}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin.dat"), data, 0o600))

	fc, err := content.Read(dir, "bin.dat")
	require.NoError(t, err)
	assert.Equal(t, "base64", fc.Encoding)
	assert.NotEmpty(t, fc.Content)
}

func TestRead_MissingFile(
	t *testing.T,
) {
	dir := t.TempDir()
	_, err := content.Read(dir, "missing.txt")
	require.Error(t, err)
}

// TestRead_LargeBinaryFile verifies that a file larger than 8 KiB containing
// a null byte in the first 8192 bytes is correctly detected as binary.
func TestRead_LargeBinaryFile(
	t *testing.T,
) {
	dir := t.TempDir()
	// Build 10 KiB of data with a null byte at position 100.
	data := bytes.Repeat([]byte("a"), 10240)
	data[100] = 0x00
	require.NoError(t, os.WriteFile(filepath.Join(dir, "large.bin"), data, 0o600))

	fc, err := content.Read(dir, "large.bin")
	require.NoError(t, err)
	assert.Equal(t, "base64", fc.Encoding)
}

// TestRead_LargeTextFile verifies that a genuinely all-text file larger than the
// old 8 KiB sample window is still returned as text (the whole-buffer scan does
// not mis-flag clean UTF-8 as binary).
func TestRead_LargeTextFile(
	t *testing.T,
) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("x"), 20000) // all ASCII, no NUL, valid UTF-8
	require.NoError(t, os.WriteFile(filepath.Join(dir, "large.txt"), data, 0o600))

	fc, err := content.Read(dir, "large.txt")
	require.NoError(t, err)
	assert.Empty(t, fc.Encoding, "all-text file must stay text")
}

// TestRegression_Read_NullBytePastSampleWindowIsBinary pins the fix for the
// leading-8-KiB sampling bug: a NUL byte beyond offset 8192 must be detected, so
// the file is base64-encoded (binary) rather than returned as text.
func TestRegression_Read_NullBytePastSampleWindowIsBinary(
	t *testing.T,
) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("x"), 8193)
	data[8192] = 0x00 // beyond the retired 8192-byte check window
	require.NoError(t, os.WriteFile(filepath.Join(dir, "late-nul.bin"), data, 0o600))

	fc, err := content.Read(dir, "late-nul.bin")
	require.NoError(t, err)
	assert.Equal(t, "base64", fc.Encoding, "NUL anywhere in the file is binary")
}

// TestRegression_Read_InvalidUTF8PastSampleWindowIsByteFaithful pins the
// silent-corruption fix. A file whose first 8 KiB is clean UTF-8 but which holds
// an invalid byte later (0xE9 — a lone Latin-1 'é') was classed as text and
// returned as a Go string; gin's encoding/json marshaller then rewrote the
// invalid byte to U+FFFD, so the client got corrupted content and a save
// persisted it. The whole-buffer scan now flags it as binary, and the base64
// payload decodes back to the EXACT original bytes.
func TestRegression_Read_InvalidUTF8PastSampleWindowIsByteFaithful(
	t *testing.T,
) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("x"), 8192)
	data = append(data, 0xE9) // invalid standalone UTF-8, past the old window
	data = append(data, []byte("trailer")...)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "latin1.txt"), data, 0o600))

	fc, err := content.Read(dir, "latin1.txt")
	require.NoError(t, err)
	require.Equal(t, "base64", fc.Encoding, "invalid UTF-8 anywhere makes the file binary")

	decoded, err := base64.StdEncoding.DecodeString(fc.Content)
	require.NoError(t, err)
	assert.Equal(t, data, decoded, "base64 round-trip must be byte-identical to the original")
}

// TestRead_PermissionDenied exercises readWithCap's os.Open error path: Stat
// succeeds (it doesn't require read permission on the file itself) but the
// subsequent os.Open fails because the file has no read permission.
func TestRead_PermissionDenied(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission semantics")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the unreadable-file permission")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o600))
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	_, err := content.Read(dir, "secret.txt")
	require.Error(t, err)
}

// TestRead_DirectoryPath exercises readWithCap's io.ReadAll error path: Stat
// and Open both succeed against a directory on Unix, but reading from it fails
// with "is a directory".
func TestRead_DirectoryPath(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))

	_, err := content.Read(dir, "sub")
	require.Error(t, err)
}

// TestRegression_Read_RejectsFileThatGrowsPastCapAfterStat pins the
// TOCTOU guard readWithCap's doc comment describes: Stat can only see a file's
// size at one instant, so the actual read is independently bounded by
// LimitReader(cap+1) — a file that grows between Stat and the read must still
// be rejected rather than silently truncated or accepted. A FIFO makes this
// deterministic without a real race: Stat reports a pipe's size as 0 (always
// under the injected cap), and opening one end blocks until the other end
// opens, so the writer goroutine's ordering relative to the read never matters.
func TestRegression_Read_RejectsFileThatGrowsPastCapAfterStat(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("named pipes are unix-only")
	}
	dir := t.TempDir()
	fifo := filepath.Join(dir, "growing")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))

	done := make(chan struct{})
	go func() {
		defer close(done)
		w, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		defer func() { _ = w.Close() }()
		// Comfortably past the 4-byte injected cap regardless of how many bytes
		// the capped read actually consumes.
		_, _ = w.Write(make([]byte, 10))
	}()
	t.Cleanup(func() { <-done })

	_, err := content.ReadWithCap(dir, "growing", 4)
	require.Error(t, err)
	require.ErrorIs(t, err, content.ErrFileTooLarge,
		"a file that grew past the cap after Stat must still be rejected as oversize")
}

func TestWrite_CreatesFile(
	t *testing.T,
) {
	dir := t.TempDir()
	err := content.Write(dir, "new.txt", "content here", "")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content here", string(data))
}

func TestWrite_CreatesParentDirs(
	t *testing.T,
) {
	dir := t.TempDir()
	err := content.Write(dir, "a/b/c.txt", "nested", "")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "a/b/c.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested", string(data))
}

func TestReadWrite_RoundTrip(
	t *testing.T,
) {
	dir := t.TempDir()
	original := "hello\nworld\n"
	require.NoError(t, content.Write(dir, "round.txt", original, ""))

	fc, err := content.Read(dir, "round.txt")
	require.NoError(t, err)
	assert.Equal(t, original, fc.Content)
}

// TestWrite_Base64ByteFaithful proves a base64-encoded write lands byte-identical
// on disk — the upload counterpart of the byte-faithful copy verb. Bytes that are
// invalid UTF-8 and contain NULs would corrupt if written through the UTF-8 text
// path; here they survive because encoding "base64" is decoded before the write.
func TestWrite_Base64ByteFaithful(
	t *testing.T,
) {
	dir := t.TempDir()
	binary := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0xFF, 0xFE, 0x01}
	encoded := base64.StdEncoding.EncodeToString(binary)

	require.NoError(t, content.Write(dir, "img.png", encoded, "base64"))

	got, err := os.ReadFile(filepath.Join(dir, "img.png"))
	require.NoError(t, err)
	assert.Equal(t, binary, got)
}

// TestWrite_Base64RoundTripsThroughRead proves the write/read pair is symmetric:
// a binary payload written as base64 reads back with Encoding "base64" and the
// same content string, so a client can round-trip a binary file losslessly.
func TestWrite_Base64RoundTripsThroughRead(
	t *testing.T,
) {
	dir := t.TempDir()
	binary := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
	encoded := base64.StdEncoding.EncodeToString(binary)

	require.NoError(t, content.Write(dir, "bin.dat", encoded, "base64"))

	fc, err := content.Read(dir, "bin.dat")
	require.NoError(t, err)
	assert.Equal(t, "base64", fc.Encoding)
	assert.Equal(t, encoded, fc.Content)
}

// TestWrite_InvalidBase64Rejected verifies a malformed base64 payload is a hard
// error (no partial file written), so a corrupt upload fails loudly.
func TestWrite_InvalidBase64Rejected(
	t *testing.T,
) {
	dir := t.TempDir()
	err := content.Write(dir, "bad.dat", "not!valid!base64", "base64")
	require.Error(t, err)
	_, statErr := os.Stat(filepath.Join(dir, "bad.dat"))
	assert.True(t, os.IsNotExist(statErr), "no file should be written on a decode error")
}

// TestWrite_UnsupportedEncodingRejected verifies an unknown encoding is rejected
// rather than silently written as text.
func TestWrite_UnsupportedEncodingRejected(
	t *testing.T,
) {
	dir := t.TempDir()
	err := content.Write(dir, "x.txt", "data", "rot13")
	require.Error(t, err)
}

// TestWrite_MkdirError exercises the os.MkdirAll error path by placing a
// regular file where a directory must be created.
func TestWrite_MkdirError(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod restrictions differ on windows")
	}
	dir := t.TempDir()
	// "parent" exists as a file — MkdirAll cannot use it as a directory.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "parent"), []byte("x"), 0o600))

	err := content.Write(dir, "parent/child.txt", "hello", "")
	require.Error(t, err)
}

// TestWrite_WriteFileError exercises the os.WriteFile error path by writing
// to a read-only directory.
func TestWrite_WriteFileError(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod restrictions differ on windows")
	}
	dir := t.TempDir()
	roDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.MkdirAll(roDir, 0o700))
	require.NoError(t, os.Chmod(roDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	err := content.Write(dir, "readonly/file.txt", "hello", "")
	require.Error(t, err)
}

// TestRegression_Write_LeavesNoTempCruft verifies the atomic write leaves no
// leftover temp file after a SUCCESSFUL write — only the target lands in the dir.
func TestRegression_Write_LeavesNoTempCruft(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, content.Write(dir, "file.txt", "payload", ""))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"file.txt"}, names, "no .tmp-* temp file may survive a successful write")
}

// TestRegression_Write_PreservesOriginalAndTempOnFailure verifies the atomic
// write's failure guarantee: when the rename-over target cannot be replaced
// (here the destination is an existing directory), the write fails, the original
// tree is untouched, and no temp cruft is left behind. os.WriteFile's O_TRUNC
// path could not offer this — it destroyed the target before writing.
func TestRegression_Write_PreservesOriginalAndTempOnFailure(
	t *testing.T,
) {
	dir := t.TempDir()
	// "occupied" already exists as a directory; renaming a temp FILE over a
	// directory fails, exercising the failure path after the temp is created.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "occupied"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "occupied", "keep.txt"), []byte("keep"), 0o600))

	err := content.Write(dir, "occupied", "clobber", "")
	require.Error(t, err, "writing over an existing directory must fail")

	// The directory and its content survived.
	kept, readErr := os.ReadFile(filepath.Join(dir, "occupied", "keep.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, []byte("keep"), kept)

	// No temp cruft left in the root.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp-", "no temp file may survive a failed write")
	}
}

// TestRegression_Write_PreservesExecutableBit verifies the atomic write carries
// an existing file's permission bits onto the replacement. Saving an executable
// script (0o755) must not silently strip its +x — the temp+rename creates a fresh
// inode, so the mode has to be copied across (os.WriteFile kept it implicitly).
func TestRegression_Write_PreservesExecutableBit(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission semantics")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho hi\n"), 0o755))

	require.NoError(t, content.Write(dir, "run.sh", "#!/bin/sh\necho bye\n", ""))

	info, err := os.Stat(script)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(), "executable bit must survive a save")

	data, err := os.ReadFile(script)
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/sh\necho bye\n", string(data))
}

// TestRegression_Write_ThroughSymlinkKeepsLink verifies a save to a symlinked
// path (whose target is inside the workspace) writes THROUGH the link, updating
// the target and leaving the symlink in place — rather than replacing the link
// with a regular file as a naive temp+rename would.
func TestRegression_Write_ThroughSymlinkKeepsLink(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target.txt"), []byte("old"), 0o600))
	require.NoError(t, os.Symlink("target.txt", filepath.Join(dir, "link.txt")))

	require.NoError(t, content.Write(dir, "link.txt", "new", ""))

	// The link is still a link, and it resolved to the updated target.
	fi, err := os.Lstat(filepath.Join(dir, "link.txt"))
	require.NoError(t, err)
	assert.NotZero(t, fi.Mode()&os.ModeSymlink, "link.txt must still be a symlink")

	target, err := os.ReadFile(filepath.Join(dir, "target.txt"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(target), "write must go through the link to its target")
}

// TestRegression_ContentRead_RejectsOversizeFile verifies that content.Read
// rejects files above the cap with ErrFileTooLarge and that files below the
// cap are still returned correctly (hardening finding R16).
// A 4-byte cap is injected via content.ReadWithCap so no 25 MiB file needs to
// exist on disk.
func TestRegression_ContentRead_RejectsOversizeFile(
	t *testing.T,
) {
	dir := t.TempDir()

	// File that exceeds the injected cap (5 bytes > 4-byte cap).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.txt"), []byte("hello"), 0o600))
	_, err := content.ReadWithCap(dir, "big.txt", 4)
	require.Error(t, err)
	require.ErrorIs(t, err, content.ErrFileTooLarge, "oversized file must return ErrFileTooLarge")

	// File that fits within the injected cap (3 bytes <= 4-byte cap).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "small.txt"), []byte("hi\n"), 0o600))
	fc, err := content.ReadWithCap(dir, "small.txt", 4)
	require.NoError(t, err, "file within cap must succeed")
	assert.Equal(t, "hi\n", fc.Content)
}

// escapeCases are the adversarial workspace-relative paths every fs op must
// reject (security finding R1).
var escapeCases = []struct {
	name string
	path string
}{
	{"parent traversal", "../../etc/passwd"},
	{"absolute path", "/etc/passwd"},
	{"mid-path escape", "a/../../b"},
}

// TestRegression_Read_RejectsPathEscape verifies content.Read refuses any path
// that escapes the workspace root, so it cannot read arbitrary host files.
func TestRegression_Read_RejectsPathEscape(
	t *testing.T,
) {
	root := t.TempDir()
	// A real secret outside the root to prove it is never returned.
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "passwd"), []byte("root:x:0:0"), 0o600))

	for _, tc := range escapeCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := content.Read(root, tc.path)
			require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
		})
	}
}

// TestRegression_Read_AllowsInWorkspace confirms a normal in-workspace read
// still works after the containment guard.
func TestRegression_Read_AllowsInWorkspace(
	t *testing.T,
) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "ok.txt"), []byte("hi"), 0o600))

	fc, err := content.Read(root, "ok.txt")
	require.NoError(t, err)
	assert.Equal(t, "hi", fc.Content)
}

// TestRegression_Write_RejectsPathEscape verifies content.Write refuses to
// write outside the workspace root (arbitrary host write / RCE vector).
func TestRegression_Write_RejectsPathEscape(
	t *testing.T,
) {
	root := t.TempDir()
	outside := t.TempDir()

	for _, tc := range escapeCases {
		t.Run(tc.name, func(t *testing.T) {
			err := content.Write(root, tc.path, "pwned", "")
			require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
		})
	}

	// Nothing was written outside the root.
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestRegression_Write_RejectsSymlinkEscape verifies content.Write cannot
// follow a symlinked parent that points outside the workspace.
func TestRegression_Write_RejectsSymlinkEscape(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	outside := t.TempDir()
	root := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "link")))

	err := content.Write(root, "link/escaped.txt", "pwned", "")
	require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)

	_, statErr := os.Stat(filepath.Join(outside, "escaped.txt"))
	assert.True(t, os.IsNotExist(statErr), "write must not land outside the root via symlink")
}

// TestRegression_Write_AllowsInWorkspace confirms a normal in-workspace write
// still works after the containment guard.
func TestRegression_Write_AllowsInWorkspace(
	t *testing.T,
) {
	root := t.TempDir()
	require.NoError(t, content.Write(root, "sub/ok.txt", "data", ""))

	data, err := os.ReadFile(filepath.Join(root, "sub/ok.txt"))
	require.NoError(t, err)
	assert.Equal(t, "data", string(data))
}
