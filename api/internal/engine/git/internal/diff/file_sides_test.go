package diff_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

func sideRepo(t *testing.T) (dir, base string) {
	t.Helper()
	dir = initRepo(t)
	writeFile(t, dir, "keep.txt", "a\nb\nc\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	return dir, headSHA(t, dir)
}

func TestFileSides_BranchDiffReadsBaseAndWorktree(t *testing.T) {
	dir, base := sideRepo(t)
	writeFile(t, dir, "keep.txt", "a\nB\nc\n")

	sides, err := diff.FileSides(context.Background(), dir, base, "keep.txt", "")
	require.NoError(t, err)

	require.NotNil(t, sides.Old)
	require.NotNil(t, sides.New)
	assert.Equal(t, "a\nb\nc\n", *sides.Old)
	assert.Equal(t, "a\nB\nc\n", *sides.New, "the new side is the uncommitted working tree")
}

func TestFileSides_CommitRangeReadsBothCommits(t *testing.T) {
	dir, base := sideRepo(t)
	writeFile(t, dir, "keep.txt", "a\nB\nc\n")
	mustGit(t, dir, "commit", "-am", "edit")
	writeFile(t, dir, "keep.txt", "uncommitted\n")

	sides, err := diff.FileSides(context.Background(), dir, base+"..HEAD", "keep.txt", "")
	require.NoError(t, err)

	assert.Equal(t, "a\nb\nc\n", *sides.Old)
	assert.Equal(t, "a\nB\nc\n", *sides.New, "a commit-scoped read ignores the working tree")
}

func TestFileSides_AddedAndDeletedFilesHaveANilSide(t *testing.T) {
	dir, base := sideRepo(t)
	writeFile(t, dir, "fresh.txt", "x\n")
	require.NoError(t, os.Remove(filepath.Join(dir, "keep.txt")))

	added, err := diff.FileSides(context.Background(), dir, base, "fresh.txt", "")
	require.NoError(t, err)
	assert.Nil(t, added.Old)
	assert.Equal(t, "x\n", *added.New)

	deleted, err := diff.FileSides(context.Background(), dir, base, "keep.txt", "")
	require.NoError(t, err)
	assert.Equal(t, "a\nb\nc\n", *deleted.Old)
	assert.Nil(t, deleted.New)
}

func TestFileSides_RenameReadsTheOldSideFromTheOldPath(t *testing.T) {
	dir, base := sideRepo(t)
	require.NoError(t, os.Rename(filepath.Join(dir, "keep.txt"), filepath.Join(dir, "moved.txt")))

	sides, err := diff.FileSides(context.Background(), dir, base, "moved.txt", "keep.txt")
	require.NoError(t, err)

	assert.Equal(t, "a\nb\nc\n", *sides.Old)
	assert.Equal(t, "a\nb\nc\n", *sides.New)
}

func TestFileSides_RefusesWhatItCannotReadSafely(t *testing.T) {
	dir, base := sideRepo(t)
	writeFile(t, dir, "big.txt", strings.Repeat("x", diff.MaxSideBytes+1))
	require.NoError(t, os.Symlink("/etc/hosts", filepath.Join(dir, "link.txt")))
	ctx := context.Background()

	_, err := diff.FileSides(ctx, dir, base, "big.txt", "")
	require.ErrorIs(t, err, diff.ErrFileTooLarge)

	_, err = diff.FileSides(ctx, dir, base, "link.txt", "")
	require.Error(t, err, "a symlink would be followed out of the repository")

	_, err = diff.FileSides(ctx, dir, base, "../outside.txt", "")
	require.ErrorIs(t, err, diff.ErrNonLocalPath)

	_, err = diff.FileSides(ctx, dir, base, "", "")
	assert.ErrorIs(t, err, diff.ErrEmptyFilePath)
}
