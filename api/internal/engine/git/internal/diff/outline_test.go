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

// TestOutline_SingleHunkFile pins the omitted-count form of the @@ header:
// `@@ -1 +1 @@` is what git emits for a one-line hunk, and a parser that
// requires "start,count" on both sides reads it as zero-length.
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

// TestOutline_MultiHunkFile asserts the exact @@ ranges of a file whose changes
// are far enough apart that -U3 keeps them in separate hunks. The line counts in
// a header cover context as well as changed lines, which is precisely the number
// the client needs to reserve scroll space.
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

// TestOutline_Rename covers `git diff -M`'s rename form, where the header names
// two different paths: the outline must carry the NEW path in Path (the one the
// client addresses the patch endpoint with) and the source in OldPath.
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

// TestOutline_BinaryFile: git emits no @@ headers at all for a binary file, so
// the entry must still appear (the client renders a placeholder row for it) with
// IsBinary set and no hunks.
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

// TestOutline_DeletedFile pins the `@@ -1,2 +0,0 @@` shape: a zero new-side
// start and count is legal and must survive the parse as zero rather than as a
// defaulted 1.
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

// TestOutline_LineLongerThanScannerBuffer is the failure mode that only ever
// shows up on real data: bufio.Scanner's default 64 KB token limit stops the
// scan dead on a minified line, and everything after it — every remaining file
// in the diff — silently vanishes from the outline. The trailing file is the
// assertion that matters.
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

// TestOutline_DensityGuard: a file whose hunk count runs away is the shape that
// makes the outline O(lines) instead of O(hunks), so collection stops at
// MaxOutlineHunksPerFile and the entry is flagged partial. The scan must carry
// on to the next file rather than abandoning the diff.
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

// TestOutline_MatchesPatchAndNumstat is the equivalence oracle. It re-derives
// every shape from the raw `git diff -U3` body — counting the lines each hunk
// actually contains rather than trusting its header — and cross-checks the
// per-file +/- totals of that walk against `git diff --numstat`. Agreement pins
// the outline parser to git's own accounting instead of to expectations written
// alongside it.
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

// TestOutline_BlankContextLine covers a context line that is truly empty: git
// renders it with no leading space at all (there is nothing to prefix), which
// is the one case body() must decrement both hunk counters from a zero-length
// line rather than mistaking it for the hunk having ended.
func TestOutline_BlankContextLine(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "f.txt", "a\nb\n\nc\nd\n")
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)
	writeFile(t, repo, "f.txt", "a\nb\n\nC\nd\n")

	files, err := diff.Outline(context.Background(), repo, ref)
	require.NoError(t, err)
	require.Len(t, files, 1)

	// A miscounted blank context line closes the hunk early and reopens a
	// second, spurious one from what remains — one correctly-sized hunk is the
	// proof the blank line was consumed as context, not as a boundary.
	assert.Equal(t, []gitdomain.HunkShape{
		{OldStart: 1, OldLines: 5, NewStart: 1, NewLines: 5},
	}, files[0].Hunks)
}

// TestOutline_NonexistentRepoPath_ReturnsStartError covers the branch before
// any streaming begins: GitStream failing to even start the subprocess (a
// nonexistent working directory makes the underlying chdir fail synchronously
// on Start), distinct from a started process that later exits non-zero.
func TestOutline_NonexistentRepoPath_ReturnsStartError(t *testing.T) {
	_, err := diff.Outline(context.Background(), "/nonexistent/path/xyz123", "HEAD")

	require.Error(t, err)
}

// TestOutline_InvalidRef_ReturnsWaitError covers Outline's wait() error
// branch: an unresolvable ref makes `git diff` itself exit non-zero, which
// only surfaces once the stream is drained and the subprocess is waited on.
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

// outlineLetterLines builds a 20-line file. Every letter named in changed is
// upper-cased, which at -U3 yields one hunk per changed letter as long as the
// letters are more than six apart.
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

// outlineSpacedLines builds a file with n changes ten lines apart, which -U3
// keeps as n separate hunks (their three-line contexts do not touch).
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

// outlineReference derives the outline from the raw patch body — the hunk line
// counts come from counting the lines in each hunk, not from re-reading the
// header the production parser reads — and totals the +/- lines per file so the
// caller can pin them against git numstat.
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

// outlineReferenceStarts reads only the two start offsets from a header; the
// counts are rebuilt from the body by outlineReferenceBody.
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
