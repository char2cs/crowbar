package diff_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// TestFileSummaries_FullBranchPicture drives FileSummaries against a real repo
// with committed branch work AND working-tree edits, proving the name-status +
// numstat merge classifies every case (modify/add/delete/rename/binary) with
// the right +/- counts, and that untracked files (no diff vs the fork point)
// are excluded — the caller folds those in from git status.
func TestFileSummaries_FullBranchPicture(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "keep.txt", "a\nb\nc\n")
	writeFile(t, dir, "torename.txt", "one\ntwo\n")
	writeFile(t, dir, "todelete.txt", "x\ny\n")
	writeFile(t, dir, "logo.bin", "\x00\x01\x02\x03")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	fork := headSHA(t, dir)

	// Committed branch work: modify, rename, delete, add, change binary.
	writeFile(t, dir, "keep.txt", "a\nb\nc\nd\ne\n")
	mustGit(t, dir, "mv", "torename.txt", "renamed.txt")
	mustGit(t, dir, "rm", "todelete.txt")
	writeFile(t, dir, "added.txt", "new1\nnew2\nnew3\n")
	writeFile(t, dir, "logo.bin", "\x09\x08\x07\x06\x05")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "branch work")

	// Working-tree edits on top of the committed work, plus a plain untracked file.
	writeFile(t, dir, "keep.txt", "a\nb\nc\nd\ne\nf\n")
	writeFile(t, dir, "untracked.txt", "not\ntracked\n")

	files, err := diff.FileSummaries(context.Background(), dir, fork)
	require.NoError(t, err)

	byPath := make(map[string]gitdomain.ReviewFileSummary, len(files))
	for _, f := range files {
		byPath[f.Path] = f
	}

	require.Contains(t, byPath, "keep.txt")
	assert.Equal(t, gitdomain.GitFileStatusModified, byPath["keep.txt"].Status)
	assert.Equal(t, 3, byPath["keep.txt"].Additions, "committed + working-tree additions vs fork")
	assert.Equal(t, 0, byPath["keep.txt"].Deletions)

	require.Contains(t, byPath, "added.txt")
	assert.Equal(t, gitdomain.GitFileStatusAdded, byPath["added.txt"].Status)
	assert.Equal(t, 3, byPath["added.txt"].Additions)

	require.Contains(t, byPath, "todelete.txt")
	assert.Equal(t, gitdomain.GitFileStatusDeleted, byPath["todelete.txt"].Status)
	assert.Equal(t, 2, byPath["todelete.txt"].Deletions)

	require.Contains(t, byPath, "renamed.txt")
	assert.Equal(t, gitdomain.GitFileStatusRenamed, byPath["renamed.txt"].Status)
	assert.Equal(t, "torename.txt", byPath["renamed.txt"].OldPath)

	require.Contains(t, byPath, "logo.bin")
	assert.Equal(t, -1, byPath["logo.bin"].Additions, "binary file carries -1 counts")
	assert.Equal(t, -1, byPath["logo.bin"].Deletions)

	assert.NotContains(t, byPath, "untracked.txt", "untracked files have no diff vs fork; excluded here")
}

// TestFileSummaries_CleanTree returns an empty summary when nothing changed
// since the ref.
func TestFileSummaries_CleanTree(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "a.txt", "hello\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	head := headSHA(t, dir)

	files, err := diff.FileSummaries(context.Background(), dir, head)
	require.NoError(t, err)
	assert.Empty(t, files)
}
