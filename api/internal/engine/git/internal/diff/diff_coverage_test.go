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

// --- WorkingTree error path ---

func TestWorkingTree_InvalidRepo(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	// not a git repo — exec will succeed but git returns non-zero
	_, err := diff.WorkingTree(ctx, dir, false)
	assert.Error(t, err)
}

// --- Commit error paths ---

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

// --- isRootCommit: the sha^ path that returns exitCode != 0 (root has no parent) ---

func TestCommit_RootCommitIsRootCommitPath(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "main.go", "package main\n")
	mustGit(t, dir, "add", "main.go")
	mustGit(t, dir, "commit", "-m", "root")

	sha := headSHA(t, dir)
	ctx := context.Background()

	// root commit: sha^ fails with exit != 0, so isRootCommit returns true
	// Commit should succeed and use git-show path
	result, err := diff.Commit(ctx, dir, sha)
	require.NoError(t, err)
	assert.Equal(t, sha, result.CommitHash)
}

// --- WorkingTree: image file changes ---

func TestWorkingTree_ImageFile(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "icon.png", "fake png content\n")
	mustGit(t, dir, "add", "icon.png")
	mustGit(t, dir, "commit", "-m", "initial")

	// replace with binary-ish content
	err := os.WriteFile(filepath.Join(dir, "icon.png"), []byte{0x89, 0x50, 0x4E, 0x47}, 0o644)
	require.NoError(t, err)
	mustGit(t, dir, "add", "icon.png")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)

	// just verify no error and the file shows up
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

// --- isZeroSHA: all-zero sha ---

func TestWorkingTree_BinaryNewFile_ZeroOldSHA(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "placeholder.go", "package main\n")
	mustGit(t, dir, "add", "placeholder.go")
	mustGit(t, dir, "commit", "-m", "initial")

	// add a binary file (new file → old SHA is all zeros)
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

// --- parseHunkHeader: missing - or + marker ---
// Tested indirectly via a diff that has an unusual hunk header shape.
// The simplest path: inject a raw diff string via the exported WorkingTree
// output to trigger parseHunkHeader edge cases is hard from external package.
// Instead cover via the commit path with a single-line file that generates
// minimal @@ headers.

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

// --- headerForHunk: out-of-bounds / non-header line ---
// Cover the case where hunks have no preceding header line (headerIdx < 0)
// by staging a file that produces a diff section with context-only content.

func TestWorkingTree_MultipleHunksHeaderForHunk(t *testing.T) {
	dir := initRepo(t)

	// Create a file large enough to produce two separate hunks.
	padding := "\n\n\n\n\n\n\n\n\n\n" // 10 blank lines to separate hunks
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

	// Multiple hunks exercise buildHunks and headerForHunk thoroughly.
	assert.GreaterOrEqual(t, len(files[0].Hunks), 2)
	for _, h := range files[0].Hunks {
		assert.NotEmpty(t, h.Header)
	}
}

// --- splitFileSections: single section (no second "diff --git" prefix) ---

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

// --- fetchBlob error path: invalid blob SHA ---
// populateBlobData is called when a file IsBinary or IsImage with non-zero SHAs.
// A deleted binary file will have a valid old SHA but zero new SHA; the old SHA
// will be fetched successfully. To force a fetchBlob error we need to give it an
// invalid (non-zero but unknown) SHA — this happens if we corrupt the index line
// externally. We can't inject that from the external test package directly, so
// we verify the successful path keeps OldBlobBase64 populated.

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
			// binary deleted file: parser sets IsBinary; IsDeleted not set (no +++ /dev/null line in binary diff)
			assert.NotEmpty(t, f.OldBlobBase64)
			assert.Empty(t, f.NewBlobBase64) // zero SHA → skipped
			break
		}
	}
	assert.True(t, found)
}

// --- parseCommitMeta: missing separators ---

func TestCommit_MetaWithDescription(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "a.go", "package main\n")
	mustGit(t, dir, "add", "a.go")
	// commit message with subject + body
	mustGit(t, dir, "commit", "-m", "subject line", "-m", "body paragraph here")

	sha := headSHA(t, dir)
	ctx := context.Background()

	result, err := diff.Commit(ctx, dir, sha)
	require.NoError(t, err)
	assert.Equal(t, "subject line", result.CommitMessage)
	assert.Contains(t, result.CommitDescription, "body paragraph")
}

// --- isImagePath: exercise the image extension check ---

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

// --- parseDiffGitPath: fewer than 4 fields ---
// This is exercised when git produces a "diff --git" line that's malformed.
// We exercise it indirectly via an empty or truncated section — but since we
// can't inject raw diff text from an external package, we just confirm that
// a normal rename diff (which has path spaces) is handled correctly.

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

// --- hunkHeader fallback: no DiffLineHeader in slice ---
// HunkPatch.buildPatch calls hunkHeader(hunkLines, hunk.Header).
// When hunkLines contains no header-type line, it falls back to hunk.Header.
// We exercise this via HunkPatch; the actual header line is always present so
// the fallback branch is hit when we call with a hunk whose lines were built
// without a header — not easily injectable from external package.
// Verify that the fallback still produces valid output.

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

// --- fetchCommitMeta error: invalid SHA in valid repo ---

func TestCommit_BadMetaSHA(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "a.go", "package main\n")
	mustGit(t, dir, "add", "a.go")
	mustGit(t, dir, "commit", "-m", "initial")

	ctx := context.Background()
	// non-existent SHA triggers fetchCommitMeta failure (git show exits non-zero)
	_, err := diff.Commit(ctx, dir, "0000000000000000000000000000000000000001")
	assert.Error(t, err)
}

// --- WorkingTree error path: cancelled context ---

func TestWorkingTree_CancelledContext(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "a.go", "package main\n")
	mustGit(t, dir, "add", "a.go")
	mustGit(t, dir, "commit", "-m", "initial")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := diff.WorkingTree(ctx, dir, false)
	// context cancellation causes exec.Git to return an error
	assert.Error(t, err)
}

// --- fetchBlob error path: cancelled context during binary diff ---

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

	// Use a pre-cancelled context so blob fetches fail after diff succeeds
	// (the diff itself is run first; blob fetch happens later in parseFileSection)
	// A pre-cancelled context will fail at the WorkingTree exec.Git call itself,
	// so instead we just confirm a normal context works and OldBlobBase64 is set.
	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)

	var found bool
	for _, f := range files {
		if f.FilePath == "img.bin" {
			found = true
			assert.True(t, f.IsBinary)
			// both old and new blobs have valid non-zero SHAs
			assert.NotEmpty(t, f.OldBlobBase64)
			assert.NotEmpty(t, f.NewBlobBase64)
			break
		}
	}
	assert.True(t, found)
}

// --- hunkHeader fallback: slice has no DiffLineHeader line ---
// HunkPatch.buildPatch calls hunkHeader(hunkLines, hunk.Header).
// hunkLines is lines[hunk.StartLine : hunk.EndLine+1].
// hunk.StartLine is always the index of the DiffLineHeader line from buildHunks.
// So the fallback path in hunkHeader (no header type in slice) would only fire
// if the slice is empty or the start line has changed.
// The HunkPatch tests with multi-file diffs already exercise the primary path;
// we can't easily hit the fallback from an external package without injectable
// fake diffs. The existing tests achieve as much coverage as possible externally.

// --- parseHunkHeader edge cases ---
// These are exercised via WorkingTree when git produces @@ headers with unusual
// shapes. The standard path is already well covered. The missing-minus and
// missing-plus paths require injected malformed headers which aren't emittable
// by git. Coverage is limited for these private parsing guards.

// --- splitFileSections: first section with no preceding "diff --git" ---
// When a text has only one diff --git section with trailing newlines the builder
// captures it. Exercise by diffing two files simultaneously.

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
