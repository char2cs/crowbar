package diff_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func TestOutline_SingleHunkFile(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "single.txt", "one\n")
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)
	writeFile(t, repo, "single.txt", "two\n")

	files, err := diff.Outline(context.Background(), repo, ref)
	require.NoError(t, err)
	require.Len(t, files, 1)

	assert.Equal(t, "single.txt", files[0].Path)
	assert.Empty(t, files[0].OldPath)
	assert.False(t, files[0].IsBinary)
	assert.False(t, files[0].IsPartial)
	assert.Equal(t, []gitdomain.HunkShape{
		{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1},
	}, files[0].Hunks)
}

func TestOutline_MultiHunkFile(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "multi.txt", outlineLetterLines(""))
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)
	writeFile(t, repo, "multi.txt", outlineLetterLines("BM"))

	files, err := diff.Outline(context.Background(), repo, ref)
	require.NoError(t, err)
	require.Len(t, files, 1)

	assert.Equal(t, []gitdomain.HunkShape{
		{OldStart: 1, OldLines: 5, NewStart: 1, NewLines: 5},
		{OldStart: 10, OldLines: 7, NewStart: 10, NewLines: 7},
	}, files[0].Hunks)
}

func TestOutline_Rename(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "torename.txt", "ren1\nren2\nren3\nren4\nren5\n")
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)
	mustGit(t, repo, "mv", "torename.txt", "renamed.txt")
	writeFile(t, repo, "renamed.txt", "ren1\nren2\nren3\nren4\nCHANGED\n")

	files, err := diff.Outline(context.Background(), repo, ref)
	require.NoError(t, err)
	require.Len(t, files, 1)

	assert.Equal(t, "renamed.txt", files[0].Path)
	assert.Equal(t, "torename.txt", files[0].OldPath)
	assert.Equal(t, []gitdomain.HunkShape{
		{OldStart: 2, OldLines: 4, NewStart: 2, NewLines: 4},
	}, files[0].Hunks)
}

func TestOutline_BinaryFile(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "logo.bin", "\x00\x01\x02\x03")
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)
	writeFile(t, repo, "logo.bin", "\x09\x08\x07\x06\x05")

	files, err := diff.Outline(context.Background(), repo, ref)
	require.NoError(t, err)
	require.Len(t, files, 1)

	assert.Equal(t, "logo.bin", files[0].Path)
	assert.True(t, files[0].IsBinary)
	assert.Empty(t, files[0].Hunks)
}

func TestOutline_DeletedFile(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "todelete.txt", "x\ny\n")
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)
	mustGit(t, repo, "rm", "todelete.txt")

	files, err := diff.Outline(context.Background(), repo, ref)
	require.NoError(t, err)
	require.Len(t, files, 1)

	assert.Equal(t, "todelete.txt", files[0].Path)
	assert.Equal(t, []gitdomain.HunkShape{
		{OldStart: 1, OldLines: 2, NewStart: 0, NewLines: 0},
	}, files[0].Hunks)
}

func TestOutline_AddedFile(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "keep.txt", "k\n")
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)
	writeFile(t, repo, "added.txt", "n1\nn2\nn3\n")
	mustGit(t, repo, "add", "added.txt")

	files, err := diff.Outline(context.Background(), repo, ref)
	require.NoError(t, err)
	require.Len(t, files, 1)

	assert.Equal(t, "added.txt", files[0].Path)
	assert.Equal(t, []gitdomain.HunkShape{
		{OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 3},
	}, files[0].Hunks)
}

func TestOutline_EmptyDiff(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "keep.txt", "k\n")
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)

	files, err := diff.Outline(context.Background(), repo, ref)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestOutline_LineLongerThanScannerBuffer(t *testing.T) {
	repo := initRepo(t)
	long := strings.Repeat("a", 200*1024)
	writeFile(t, repo, "minified.js", long+"\n")
	writeFile(t, repo, "zz-after.txt", "before\n")
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)
	writeFile(t, repo, "minified.js", strings.Repeat("b", 300*1024)+"\n")
	writeFile(t, repo, "zz-after.txt", "after\n")

	files, err := diff.Outline(context.Background(), repo, ref)
	require.NoError(t, err)

	byPath := outlineByPath(files)
	require.Contains(t, byPath, "minified.js")
	assert.Equal(t, []gitdomain.HunkShape{
		{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1},
	}, byPath["minified.js"].Hunks)

	require.Contains(t, byPath, "zz-after.txt", "scanning must survive a line past the buffer size")
	assert.Equal(t, []gitdomain.HunkShape{
		{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1},
	}, byPath["zz-after.txt"].Hunks)
}

func TestOutline_DensityGuard(t *testing.T) {
	repo := initRepo(t)
	const hunks = diff.MaxOutlineHunksPerFile + 5
	writeFile(t, repo, "dense.txt", outlineSpacedLines(hunks, false))
	writeFile(t, repo, "zz-after.txt", "before\n")
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)
	writeFile(t, repo, "dense.txt", outlineSpacedLines(hunks, true))
	writeFile(t, repo, "zz-after.txt", "after\n")

	files, err := diff.Outline(context.Background(), repo, ref)
	require.NoError(t, err)

	byPath := outlineByPath(files)
	require.Contains(t, byPath, "dense.txt")
	dense := byPath["dense.txt"]
	assert.True(t, dense.IsPartial, "a file past the cap is partial")
	assert.Len(t, dense.Hunks, diff.MaxOutlineHunksPerFile, "hunks are capped, not truncated to one")
	assert.Equal(t,
		gitdomain.HunkShape{OldStart: 2, OldLines: 7, NewStart: 2, NewLines: 7},
		dense.Hunks[0],
		"the hunks kept are the first ones, in order",
	)

	require.Contains(t, byPath, "zz-after.txt", "scanning continues past a capped file")
	assert.False(t, byPath["zz-after.txt"].IsPartial)
	assert.Len(t, byPath["zz-after.txt"].Hunks, 1)
}

func TestOutline_MatchesPatchAndNumstat(t *testing.T) {
	repo := initRepo(t)
	seedEquivalenceBase(t, repo)
	writeFile(t, repo, "dense.txt", outlineSpacedLines(6, false))
	writeFile(t, repo, "single.txt", "only\n")
	commitAll(t, repo, "outline base")
	ref := headSHA(t, repo)

	writeFile(t, repo, "mod.txt", "1\n2\nX\n4\n5\n6\n7\n8\n")
	writeFile(t, repo, "dense.txt", outlineSpacedLines(6, true))
	writeFile(t, repo, "single.txt", "changed\n")
	mustGit(t, repo, "mv", "ren.txt", "renamed.txt")
	mustGit(t, repo, "rm", "del.txt")
	writeFile(t, repo, "added.txt", "a1\na2\na3\n")
	writeFile(t, repo, "logo.bin", "\x09\x08\x07")
	commitAll(t, repo, "branch work")
	writeFile(t, repo, "keep.txt", "a\nb\nc\nd\ne\nf\ng\n")

	want, bodyCounts := outlineReference(t, repo, ref)
	got, err := diff.Outline(context.Background(), repo, ref)
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.NotEmpty(t, got)

	numstat := exec.Git(context.Background(), repo, "diff", "--numstat", "-M", "-z", ref, "--")
	require.Equal(t, 0, numstat.ExitCode, numstat.Stderr)
	counts := referenceCounts(numstat.Stdout)
	for _, f := range got {
		if f.IsBinary {
			continue
		}
		require.Contains(t, counts, f.Path)
		assert.Equal(t, counts[f.Path][0], bodyCounts[f.Path][0], "additions for %s", f.Path)
		assert.Equal(t, counts[f.Path][1], bodyCounts[f.Path][1], "deletions for %s", f.Path)
	}
}

func TestOutline_BlankContextLine(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "f.txt", "a\nb\n\nc\nd\n")
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)
	writeFile(t, repo, "f.txt", "a\nb\n\nC\nd\n")

	files, err := diff.Outline(context.Background(), repo, ref)
	require.NoError(t, err)
	require.Len(t, files, 1)

	assert.Equal(t, []gitdomain.HunkShape{
		{OldStart: 1, OldLines: 5, NewStart: 1, NewLines: 5},
	}, files[0].Hunks)
}

func TestOutline_NonexistentRepoPath_ReturnsStartError(t *testing.T) {
	_, err := diff.Outline(context.Background(), "/nonexistent/path/xyz123", "HEAD")

	require.Error(t, err)
}

func TestOutline_InvalidRef_ReturnsWaitError(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "f.txt", "a\n")
	commitAll(t, repo, "base")

	_, err := diff.Outline(context.Background(), repo, "not-a-real-ref")

	require.Error(t, err)
}

func outlineByPath(
	files []gitdomain.FileOutline,
) map[string]gitdomain.FileOutline {
	out := make(map[string]gitdomain.FileOutline, len(files))
	for _, f := range files {
		out[f.Path] = f
	}
	return out
}

func outlineLetterLines(
	changed string,
) string {
	var b strings.Builder
	for i := range 20 {
		letter := string(rune('a' + i))
		if strings.Contains(changed, strings.ToUpper(letter)) {
			letter = strings.ToUpper(letter)
		}
		b.WriteString(letter + "\n")
	}
	return b.String()
}

func outlineSpacedLines(
	n int,
	changed bool,
) string {
	var b strings.Builder
	for i := range n * 10 {
		if i%10 == 4 && changed {
			b.WriteString("CHANGED\n")
			continue
		}
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

func outlineReference(
	t *testing.T,
	repo string,
	ref string,
) ([]gitdomain.FileOutline, map[string][2]int) {
	t.Helper()
	r := exec.Git(context.Background(), repo, "diff", "-M", "-U3", ref, "--")
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	var files []gitdomain.FileOutline
	counts := make(map[string][2]int)
	idx := -1
	for _, line := range strings.Split(r.Stdout, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			files = append(files, gitdomain.FileOutline{
				Path:  line[strings.LastIndex(line, " b/")+3:],
				Hunks: []gitdomain.HunkShape{},
			})
			idx = len(files) - 1
		case idx < 0:
		case strings.HasPrefix(line, "rename from "):
			files[idx].OldPath = strings.TrimPrefix(line, "rename from ")
		case strings.HasPrefix(line, "rename to "):
			files[idx].Path = strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "Binary files "):
			files[idx].IsBinary = true
		case strings.HasPrefix(line, "@@ "):
			files[idx].Hunks = append(files[idx].Hunks, outlineReferenceStarts(t, line))
		case len(files[idx].Hunks) > 0 && line != "":
			outlineReferenceBody(&files[idx], counts, line)
		}
	}
	return files, counts
}

func outlineReferenceBody(
	f *gitdomain.FileOutline,
	counts map[string][2]int,
	line string,
) {
	h := &f.Hunks[len(f.Hunks)-1]
	c := counts[f.Path]
	switch line[0] {
	case '+':
		h.NewLines++
		c[0]++
	case '-':
		h.OldLines++
		c[1]++
	case ' ':
		h.OldLines++
		h.NewLines++
	}
	counts[f.Path] = c
}

func outlineReferenceStarts(
	t *testing.T,
	line string,
) gitdomain.HunkShape {
	t.Helper()
	fields := strings.Fields(line)
	require.GreaterOrEqual(t, len(fields), 3, "malformed hunk header %q", line)
	return gitdomain.HunkShape{
		OldStart: outlineReferenceStart(t, fields[1], "-"),
		NewStart: outlineReferenceStart(t, fields[2], "+"),
	}
}

func outlineReferenceStart(
	t *testing.T,
	field string,
	sign string,
) int {
	t.Helper()
	require.True(t, strings.HasPrefix(field, sign), "hunk range %q lacks %q", field, sign)
	n, err := strconv.Atoi(strings.SplitN(strings.TrimPrefix(field, sign), ",", 2)[0])
	require.NoError(t, err)
	return n
}
