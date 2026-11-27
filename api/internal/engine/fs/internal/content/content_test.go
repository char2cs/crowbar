package content_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/content"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
	"github.com/char2cs/crowbar/api/internal/testutil"
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

func TestRead_LargeTextFile(
	t *testing.T,
) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("x"), 20000)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "large.txt"), data, 0o600))

	fc, err := content.Read(dir, "large.txt")
	require.NoError(t, err)
	assert.Empty(t, fc.Encoding, "all-text file must stay text")
}

func TestRegression_Read_NullBytePastSampleWindowIsBinary(
	t *testing.T,
) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("x"), 8193)
	data[8192] = 0x00
	require.NoError(t, os.WriteFile(filepath.Join(dir, "late-nul.bin"), data, 0o600))

	fc, err := content.Read(dir, "late-nul.bin")
	require.NoError(t, err)
	assert.Equal(t, "base64", fc.Encoding, "NUL anywhere in the file is binary")
}

func TestRegression_Read_InvalidUTF8PastSampleWindowIsByteFaithful(
	t *testing.T,
) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("x"), 8192)
	data = append(data, 0xE9)
	data = append(data, []byte("trailer")...)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "latin1.txt"), data, 0o600))

	fc, err := content.Read(dir, "latin1.txt")
	require.NoError(t, err)
	require.Equal(t, "base64", fc.Encoding, "invalid UTF-8 anywhere makes the file binary")

	decoded, err := base64.StdEncoding.DecodeString(fc.Content)
	require.NoError(t, err)
	assert.Equal(t, data, decoded, "base64 round-trip must be byte-identical to the original")
}

func TestRead_PermissionDenied(
	t *testing.T,
) {
	testutil.RequirePermissionEnforcement(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o600))
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	_, err := content.Read(dir, "secret.txt")
	require.Error(t, err)
}

func TestRead_DirectoryPath(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))

	_, err := content.Read(dir, "sub")
	require.Error(t, err)
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

func TestWrite_InvalidBase64Rejected(
	t *testing.T,
) {
	dir := t.TempDir()
	err := content.Write(dir, "bad.dat", "not!valid!base64", "base64")
	require.Error(t, err)
	_, statErr := os.Stat(filepath.Join(dir, "bad.dat"))
	assert.True(t, os.IsNotExist(statErr), "no file should be written on a decode error")
}

func TestWrite_UnsupportedEncodingRejected(
	t *testing.T,
) {
	dir := t.TempDir()
	err := content.Write(dir, "x.txt", "data", "rot13")
	require.Error(t, err)
}

func TestWrite_MkdirError(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod restrictions differ on windows")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "parent"), []byte("x"), 0o600))

	err := content.Write(dir, "parent/child.txt", "hello", "")
	require.Error(t, err)
}

func TestWrite_WriteFileError(
	t *testing.T,
) {
	testutil.RequirePermissionEnforcement(t)
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

func TestRegression_Write_PreservesOriginalAndTempOnFailure(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "occupied"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "occupied", "keep.txt"), []byte("keep"), 0o600))

	err := content.Write(dir, "occupied", "clobber", "")
	require.Error(t, err, "writing over an existing directory must fail")

	kept, readErr := os.ReadFile(filepath.Join(dir, "occupied", "keep.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, []byte("keep"), kept)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp-", "no temp file may survive a failed write")
	}
}

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

	fi, err := os.Lstat(filepath.Join(dir, "link.txt"))
	require.NoError(t, err)
	assert.NotZero(t, fi.Mode()&os.ModeSymlink, "link.txt must still be a symlink")

	target, err := os.ReadFile(filepath.Join(dir, "target.txt"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(target), "write must go through the link to its target")
}

func TestRegression_Write_RefusesADanglingSymlink(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	root := t.TempDir()
	planted := filepath.Join(t.TempDir(), "planted")
	require.NoError(t, os.Symlink(planted, filepath.Join(root, "link")))

	require.ErrorIs(t, content.Write(root, "link", "x", ""), safepath.ErrDanglingSymlink)
	require.ErrorIs(t, content.WriteAbs(filepath.Join(root, "link"), "x", ""), safepath.ErrDanglingSymlink)
	_, err := os.Lstat(planted)
	assert.ErrorIs(t, err, os.ErrNotExist, "no write may create the link's target")
}

func TestRegression_ContentRead_RejectsOversizeFile(
	t *testing.T,
) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.txt"), []byte("hello"), 0o600))
	_, err := content.ReadWithCap(dir, "big.txt", 4)
	require.Error(t, err)
	require.ErrorIs(t, err, content.ErrFileTooLarge, "oversized file must return ErrFileTooLarge")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "small.txt"), []byte("hi\n"), 0o600))
	fc, err := content.ReadWithCap(dir, "small.txt", 4)
	require.NoError(t, err, "file within cap must succeed")
	assert.Equal(t, "hi\n", fc.Content)
}

var escapeCases = []struct {
	name string
	path string
}{
	{"parent traversal", "../../etc/passwd"},
	{"absolute path", "/etc/passwd"},
	{"mid-path escape", "a/../../b"},
}

func TestRegression_Read_RejectsPathEscape(
	t *testing.T,
) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "passwd"), []byte("root:x:0:0"), 0o600))

	for _, tc := range escapeCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := content.Read(root, tc.path)
			require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
		})
	}
}

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

	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

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

func TestAbs_RejectsRelativeAndUncleanPaths(
	t *testing.T,
) {
	for _, p := range []string{"notes.md", "/tmp/a/../notes.md", "/tmp//notes.md"} {
		_, err := content.ReadAbs(p)
		require.ErrorIs(t, err, safepath.ErrNotAbsolute, p)
		require.ErrorIs(t, content.WriteAbs(p, "x", ""), safepath.ErrNotAbsolute, p)
	}
}

func TestAbs_RoundTripAndCap(
	t *testing.T,
) {
	abs := filepath.Join(t.TempDir(), "sub", "a.bin")
	encoded := base64.StdEncoding.EncodeToString([]byte{0, 0xff})
	require.NoError(t, content.WriteAbs(abs, encoded, "base64"))
	fc, err := content.ReadAbs(abs)
	require.NoError(t, err)
	assert.Equal(t, [2]string{encoded, "base64"}, [2]string{fc.Content, fc.Encoding})

	require.NoError(t, os.Truncate(abs, 25<<20+1))
	_, err = content.ReadAbs(abs)
	require.ErrorIs(t, err, content.ErrFileTooLarge)
}
