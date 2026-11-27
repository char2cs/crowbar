package diff_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

func TestWorkingTree_InvalidRepo(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	_, err := diff.WorkingTree(ctx, dir, false)
	assert.Error(t, err)
}

func TestCommit_InvalidSHA(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "main.go", "package main\n")
	mustGit(t, dir, "add", "main.go")
	mustGit(t, dir, "commit", "-m", "initial")

	ctx := context.Background()
	_, err := diff.Commit(ctx, dir, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	assert.Error(t, err)
}

func TestCommit_InvalidRepo(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	_, err := diff.Commit(ctx, dir, "deadbeefdeadbeef")
	assert.Error(t, err)
}

func TestCommit_RootCommitIsRootCommitPath(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "main.go", "package main\n")
	mustGit(t, dir, "add", "main.go")
	mustGit(t, dir, "commit", "-m", "root")

	sha := headSHA(t, dir)
	ctx := context.Background()

	result, err := diff.Commit(ctx, dir, sha)
	require.NoError(t, err)
	assert.Equal(t, sha, result.CommitHash)
}

func TestWorkingTree_ImageFile(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "icon.png", "fake png content\n")
	mustGit(t, dir, "add", "icon.png")
	mustGit(t, dir, "commit", "-m", "initial")

	err := os.WriteFile(filepath.Join(dir, "icon.png"), []byte{0x89, 0x50, 0x4E, 0x47}, 0o644)
	require.NoError(t, err)
	mustGit(t, dir, "add", "icon.png")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)

	found := false
	for _, f := range files {
		if f.FilePath == "icon.png" {
			found = true
			assert.True(t, f.IsImage)
			break
		}
	}
	assert.True(t, found, "expected icon.png in diff")
}

func TestWorkingTree_BinaryNewFile_ZeroOldSHA(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "placeholder.go", "package main\n")
	mustGit(t, dir, "add", "placeholder.go")
	mustGit(t, dir, "commit", "-m", "initial")

	err := os.WriteFile(filepath.Join(dir, "new.bin"), []byte{0x00, 0x01, 0x02, 0xFF}, 0o644)
	require.NoError(t, err)
	mustGit(t, dir, "add", "new.bin")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)

	var found bool
	for _, f := range files {
		if f.FilePath == "new.bin" {
			found = true
			assert.True(t, f.IsBinary)
			break
		}
	}
	assert.True(t, found)
}

func TestCommit_SingleLineFile(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "a.go", "package main\n")
	mustGit(t, dir, "add", "a.go")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "a.go", "package main\n\nfunc main() {}\n")
	mustGit(t, dir, "add", "a.go")
	mustGit(t, dir, "commit", "-m", "second")

	sha := headSHA(t, dir)
	ctx := context.Background()

	result, err := diff.Commit(ctx, dir, sha)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Files)
}

func TestWorkingTree_MultipleHunksHeaderForHunk(t *testing.T) {
	dir := initRepo(t)

	padding := "\n\n\n\n\n\n\n\n\n\n"
	original := "package main" + padding +
		"func a() {}" + padding +
		"func b() {}\n"
	writeFile(t, dir, "multi.go", original)
	mustGit(t, dir, "add", "multi.go")
	mustGit(t, dir, "commit", "-m", "initial")

	modified := "package main" + padding +
		"func a() { return }" + padding +
		"func b() { return }\n"
	writeFile(t, dir, "multi.go", modified)
	mustGit(t, dir, "add", "multi.go")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)
	require.Len(t, files, 1)

	assert.GreaterOrEqual(t, len(files[0].Hunks), 2)
	for _, h := range files[0].Hunks {
		assert.NotEmpty(t, h.Header)
	}
}

func TestWorkingTree_OnlyOneFile(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "only.go", "package main\n\nfunc only() {}\n")
	mustGit(t, dir, "add", "only.go")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "only.go", "package main\n\nfunc only() { return }\n")
	mustGit(t, dir, "add", "only.go")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "only.go", files[0].FilePath)
}

func TestWorkingTree_DeletedBinaryFile_HasOldBlob(t *testing.T) {
	dir := initRepo(t)

	binaryContent := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE}
	err := os.WriteFile(filepath.Join(dir, "data.bin"), binaryContent, 0o644)
	require.NoError(t, err)
	mustGit(t, dir, "add", "data.bin")
	mustGit(t, dir, "commit", "-m", "initial")

	err = os.Remove(filepath.Join(dir, "data.bin"))
	require.NoError(t, err)
	mustGit(t, dir, "add", "data.bin")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)

	var found bool
	for _, f := range files {
		if f.FilePath == "data.bin" {
			found = true
			assert.True(t, f.IsBinary)
			assert.NotEmpty(t, f.OldBlobBase64)
			assert.Empty(t, f.NewBlobBase64)
			break
		}
	}
	assert.True(t, found)
}

func TestCommit_MetaWithDescription(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "a.go", "package main\n")
	mustGit(t, dir, "add", "a.go")
	mustGit(t, dir, "commit", "-m", "subject line", "-m", "body paragraph here")

	sha := headSHA(t, dir)
	ctx := context.Background()

	result, err := diff.Commit(ctx, dir, sha)
	require.NoError(t, err)
	assert.Equal(t, "subject line", result.CommitMessage)
	assert.Contains(t, result.CommitDescription, "body paragraph")
}

func TestWorkingTree_SvgFile(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "logo.svg", "<svg></svg>\n")
	mustGit(t, dir, "add", "logo.svg")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "logo.svg", "<svg><!-- updated --></svg>\n")
	mustGit(t, dir, "add", "logo.svg")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)

	var found bool
	for _, f := range files {
		if f.FilePath == "logo.svg" {
			found = true
			assert.True(t, f.IsImage, "svg should be treated as image")
			break
		}
	}
	assert.True(t, found)
}

func TestWorkingTree_RenamedFilePathParsing(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "before.go", "package main\nfunc a() {}\nfunc b() {}\nfunc c() {}\n")
	mustGit(t, dir, "add", "before.go")
	mustGit(t, dir, "commit", "-m", "initial")

	err := os.Rename(
		filepath.Join(dir, "before.go"),
		filepath.Join(dir, "after.go"),
	)
	require.NoError(t, err)
	mustGit(t, dir, "add", "before.go", "after.go")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)

	var renamed bool
	for _, f := range files {
		if f.IsRenamed {
			renamed = true
			assert.Equal(t, "after.go", f.FilePath)
			assert.Equal(t, "before.go", f.OldPath)
			break
		}
	}
	assert.True(t, renamed)
}

func TestHunkPatch_MultipleHunks_AllPatched(t *testing.T) {
	dir := initRepo(t)

	padding := "\n\n\n\n\n\n\n\n\n\n"
	original := "package main" + padding + "func a() {}" + padding + "func b() {}\n"
	writeFile(t, dir, "multi.go", original)
	mustGit(t, dir, "add", "multi.go")
	mustGit(t, dir, "commit", "-m", "initial")

	modified := "package main" + padding + "func a() { return }" + padding + "func b() { return }\n"
	writeFile(t, dir, "multi.go", modified)

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, false)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.GreaterOrEqual(t, len(files[0].Hunks), 2)

	for _, hunk := range files[0].Hunks {
		patch, err := diff.HunkPatch(ctx, dir, "multi.go", hunk.HunkID, false)
		require.NoError(t, err)
		assert.Contains(t, patch, "diff --git")
		assert.Contains(t, patch, "@@")
	}
}

func TestCommit_BadMetaSHA(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "a.go", "package main\n")
	mustGit(t, dir, "add", "a.go")
	mustGit(t, dir, "commit", "-m", "initial")

	ctx := context.Background()
	_, err := diff.Commit(ctx, dir, "0000000000000000000000000000000000000001")
	assert.Error(t, err)
}

func TestWorkingTree_CancelledContext(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "a.go", "package main\n")
	mustGit(t, dir, "add", "a.go")
	mustGit(t, dir, "commit", "-m", "initial")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := diff.WorkingTree(ctx, dir, false)
	assert.Error(t, err)
}

func TestWorkingTree_BinaryFile_CancelledContext(t *testing.T) {
	dir := initRepo(t)

	binaryContent := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE}
	err := os.WriteFile(filepath.Join(dir, "img.bin"), binaryContent, 0o644)
	require.NoError(t, err)
	mustGit(t, dir, "add", "img.bin")
	mustGit(t, dir, "commit", "-m", "initial")

	newBinary := []byte{0x10, 0x20, 0x30, 0x40}
	err = os.WriteFile(filepath.Join(dir, "img.bin"), newBinary, 0o644)
	require.NoError(t, err)
	mustGit(t, dir, "add", "img.bin")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)

	var found bool
	for _, f := range files {
		if f.FilePath == "img.bin" {
			found = true
			assert.True(t, f.IsBinary)
			assert.NotEmpty(t, f.OldBlobBase64)
			assert.NotEmpty(t, f.NewBlobBase64)
			break
		}
	}
	assert.True(t, found)
}

func TestWorkingTree_TwoFiles(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "a.go", "package main\nfunc a() {}\n")
	writeFile(t, dir, "b.go", "package main\nfunc b() {}\n")
	mustGit(t, dir, "add", "a.go", "b.go")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "a.go", "package main\nfunc a() { return }\n")
	writeFile(t, dir, "b.go", "package main\nfunc b() { return }\n")
	mustGit(t, dir, "add", "a.go", "b.go")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)
	require.Len(t, files, 2)
}
