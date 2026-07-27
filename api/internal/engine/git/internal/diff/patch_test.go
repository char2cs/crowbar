package diff_test

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// gitPatchOracle is what `git diff` itself produces for one path, so the tests
// pin FilePatch against git rather than against a hand-written expectation.
func gitPatchOracle(
	t *testing.T,
	dir string,
	ref string,
	path string,
) string {
	t.Helper()
	r := exec.Git(context.Background(), dir, "diff", "-M", "-U3", ref, "--", ":(top,literal)"+path)
	require.Equal(t, 0, r.ExitCode, "oracle diff failed: %s", r.Stderr)
	return r.Stdout
}

// TestFilePatch_ModifiedFile is the ordinary case: one file's unified patch,
// byte-identical to git's own, with the written line count reported and no
// truncation.
func TestFilePatch_ModifiedFile(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "keep.txt", "a\nb\nc\n")
	writeFile(t, dir, "other.txt", "untouched\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	base := headSHA(t, dir)

	writeFile(t, dir, "keep.txt", "a\nB\nc\n")
	writeFile(t, dir, "other.txt", "also changed\n")

	var buf bytes.Buffer
	lines, truncated, err := diff.FilePatch(context.Background(), dir, base, "keep.txt", 0, &buf)
	require.NoError(t, err)
	assert.False(t, truncated)

	out := buf.String()
	assert.True(t, strings.HasPrefix(out, "diff --git "), "patch must start with the diff header, got %q", firstLine(out))
	assert.Equal(t, gitPatchOracle(t, dir, base, "keep.txt"), out, "patch must be byte-identical to git's own")
	assert.NotContains(t, out, "also changed", "the pathspec must scope the diff to the requested file")
	assert.Equal(t, countLines(out), lines)
}

// TestFilePatch_PathNotInDiff proves a path that simply has no changes is not
// an error — it is an empty patch, which is what a client asking for a file it
// no longer has changes in must see.
func TestFilePatch_PathNotInDiff(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "a.txt", "hello\n")
	writeFile(t, dir, "b.txt", "world\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	base := headSHA(t, dir)
	writeFile(t, dir, "a.txt", "changed\n")

	var buf bytes.Buffer
	lines, truncated, err := diff.FilePatch(context.Background(), dir, base, "b.txt", 0, &buf)
	require.NoError(t, err)
	assert.Equal(t, 0, lines)
	assert.False(t, truncated)
	assert.Empty(t, buf.String())

	var missing bytes.Buffer
	lines, truncated, err = diff.FilePatch(context.Background(), dir, base, "no/such/file.txt", 0, &missing)
	require.NoError(t, err, "a path absent from the tree entirely is still not an error")
	assert.Equal(t, 0, lines)
	assert.False(t, truncated)
	assert.Empty(t, missing.String())
}

// TestFilePatch_RenamedFileAddressedByNewPath drives the rename case the client
// reaches from the outline: the outline carries OldPath, but the patch is
// fetched by the NEW path. Scoping the diff to one path is what keeps the query
// off the O(diff size) path, and it necessarily hides the rename pairing — git
// renders the surviving half as an addition. The content must still be there.
func TestFilePatch_RenamedFileAddressedByNewPath(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "before.txt", "one\ntwo\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	base := headSHA(t, dir)

	mustGit(t, dir, "mv", "before.txt", "after.txt")
	writeFile(t, dir, "after.txt", "one\ntwo\nthree\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "rename")

	var buf bytes.Buffer
	lines, truncated, err := diff.FilePatch(context.Background(), dir, base, "after.txt", 0, &buf)
	require.NoError(t, err)
	assert.False(t, truncated)
	assert.Positive(t, lines)

	out := buf.String()
	assert.Contains(t, out, "diff --git a/after.txt b/after.txt")
	assert.Contains(t, out, "+three")
	assert.Equal(t, gitPatchOracle(t, dir, base, "after.txt"), out)
}

// TestFilePatch_BinaryFile proves a binary file yields git's marker line rather
// than its bytes — a patch endpoint that streamed raw binary into a text
// response would corrupt the client.
func TestFilePatch_BinaryFile(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "logo.bin", "\x00\x01\x02\x03")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	base := headSHA(t, dir)
	writeFile(t, dir, "logo.bin", "\x00\xff\xfe\xfd\x04")

	var buf bytes.Buffer
	lines, truncated, err := diff.FilePatch(context.Background(), dir, base, "logo.bin", 0, &buf)
	require.NoError(t, err)
	assert.False(t, truncated)
	assert.Positive(t, lines)

	out := buf.String()
	assert.Contains(t, out, "Binary files ")
	assert.Contains(t, out, " differ")
	assert.NotContains(t, out, "\xff\xfe\xfd", "binary content must never reach the writer")
}

// TestFilePatch_TruncatesAtHunkBoundary is the load-bearing case: a cut that
// lands mid-hunk would hand a client parser a hunk header promising more lines
// than follow, which produces garbage rather than an error. The cut must fall
// between hunks, and the result must still apply.
func TestFilePatch_TruncatesAtHunkBoundary(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "many.txt", numberedLines(1, 200))
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	base := headSHA(t, dir)
	writeFile(t, dir, "many.txt", editEvery(numberedLines(1, 200), 20))

	full := gitPatchOracle(t, dir, base, "many.txt")
	require.Greater(t, strings.Count(full, "\n@@ "), 3, "fixture must have several hunks")

	var buf bytes.Buffer
	lines, truncated, err := diff.FilePatch(context.Background(), dir, base, "many.txt", 30, &buf)
	require.NoError(t, err)
	assert.True(t, truncated, "a 30-line cap over a much larger patch must report truncation")

	out := buf.String()
	assert.Equal(t, countLines(out), lines)
	assert.LessOrEqual(t, lines, 30, "the cut rounds DOWN to a whole hunk, never past the cap")
	assert.True(t, strings.HasPrefix(out, "diff --git "))
	assertEveryHunkComplete(t, out)
	assertApplies(t, dir, base, out)
}

// TestFilePatch_TruncationWhenFirstHunkExceedsCap pins the degenerate shape the
// perf fixture actually has: a single hunk far larger than any cap. No whole
// hunk fits, so the only valid patch under the cap is the file header alone —
// reported as truncated so the client knows to widen or page by hunk.
func TestFilePatch_TruncationWhenFirstHunkExceedsCap(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "big.txt", "seed\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	base := headSHA(t, dir)
	writeFile(t, dir, "big.txt", "seed\n"+numberedLines(1, 500))

	var buf bytes.Buffer
	lines, truncated, err := diff.FilePatch(context.Background(), dir, base, "big.txt", 10, &buf)
	require.NoError(t, err)
	assert.True(t, truncated)

	out := buf.String()
	assert.Equal(t, countLines(out), lines)
	assert.LessOrEqual(t, lines, 10)
	assert.True(t, strings.HasPrefix(out, "diff --git "))
	assert.NotContains(t, out, "\n@@ ", "a hunk that cannot fit whole must not be emitted at all")
	assertEveryHunkComplete(t, out)
}

// TestFilePatch_NotTruncatedWhenPatchFitsExactly guards the off-by-one: a patch
// whose length equals the cap is complete, not truncated.
func TestFilePatch_NotTruncatedWhenPatchFitsExactly(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "fit.txt", "a\nb\nc\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	base := headSHA(t, dir)
	writeFile(t, dir, "fit.txt", "a\nB\nc\n")

	exact := countLines(gitPatchOracle(t, dir, base, "fit.txt"))

	var buf bytes.Buffer
	lines, truncated, err := diff.FilePatch(context.Background(), dir, base, "fit.txt", exact, &buf)
	require.NoError(t, err)
	assert.False(t, truncated, "a patch exactly the size of the cap is whole")
	assert.Equal(t, exact, lines)
	assert.Equal(t, gitPatchOracle(t, dir, base, "fit.txt"), buf.String())
}

// TestFilePatch_MaxLinesUnlimited proves a non-positive cap means no cap.
func TestFilePatch_MaxLinesUnlimited(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "wide.txt", numberedLines(1, 400))
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	base := headSHA(t, dir)
	writeFile(t, dir, "wide.txt", editEvery(numberedLines(1, 400), 10))

	want := gitPatchOracle(t, dir, base, "wide.txt")
	for _, maxLines := range []int{0, -1} {
		var buf bytes.Buffer
		lines, truncated, err := diff.FilePatch(context.Background(), dir, base, "wide.txt", maxLines, &buf)
		require.NoError(t, err, "maxLines=%d", maxLines)
		assert.False(t, truncated, "maxLines=%d", maxLines)
		assert.Equal(t, want, buf.String(), "maxLines=%d", maxLines)
		assert.Equal(t, countLines(want), lines, "maxLines=%d", maxLines)
	}
}

// TestFilePatch_PathspecHostileNames drives the names that break a naive
// pathspec. Each repo carries a decoy the requested name would also match if
// the pathspec were interpreted as magic or as a glob, so a passing assertion
// proves the :(top,literal) prefix is doing its job rather than that the diff
// happened to contain one file.
func TestFilePatch_PathspecHostileNames(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		decoy string
	}{
		{name: "pathspec magic prefix", path: ":x", decoy: "x"},
		{name: "glob metacharacters", path: "we*ird[1].txt", decoy: "weird1.txt"},
		{name: "space in name", path: "with space.txt", decoy: "withXspace.txt"},
		{name: "non-ascii", path: "café-ñ.txt", decoy: "cafe-n.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := initRepo(t)
			writeFile(t, dir, tc.path, "base\n")
			writeFile(t, dir, tc.decoy, "base\n")
			mustGit(t, dir, "add", "-A")
			mustGit(t, dir, "commit", "-m", "initial")
			base := headSHA(t, dir)
			writeFile(t, dir, tc.path, "base\nwanted-line\n")
			writeFile(t, dir, tc.decoy, "base\ndecoy-line\n")

			var buf bytes.Buffer
			lines, truncated, err := diff.FilePatch(context.Background(), dir, base, tc.path, 0, &buf)
			require.NoError(t, err)
			assert.False(t, truncated)
			assert.Positive(t, lines)

			out := buf.String()
			assert.Contains(t, out, "+wanted-line")
			assert.NotContains(t, out, "+decoy-line", "the decoy must not be swept in")
			assert.Equal(t, 1, strings.Count(out, "diff --git "), "exactly one file section")
		})
	}
}

// TestFilePatch_LineLongerThanScannerDefault is the failure mode that only
// shows up on real data: bufio.Scanner caps a token at 64 KB by default and the
// perf fixture's minified bundle carries a 657 KB line. A patch reader that
// silently drops or errors on it is useless on exactly the files a user most
// needs the windowed API for.
func TestFilePatch_LineLongerThanScannerDefault(t *testing.T) {
	const lineLen = 300 * 1024
	huge := strings.Repeat("m", lineLen)

	dir := initRepo(t)
	writeFile(t, dir, "bundle.min.js", "short\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	base := headSHA(t, dir)
	writeFile(t, dir, "bundle.min.js", "short\n"+huge+"\n")

	var buf bytes.Buffer
	lines, truncated, err := diff.FilePatch(context.Background(), dir, base, "bundle.min.js", 0, &buf)
	require.NoError(t, err)
	assert.False(t, truncated)

	out := buf.String()
	assert.Contains(t, out, "+"+huge, "the whole oversized line must survive the copy")
	assert.Equal(t, gitPatchOracle(t, dir, base, "bundle.min.js"), out)
	assert.Equal(t, countLines(out), lines)
}

// TestFilePatch_NoTrailingNewline proves a patch git ends without a final
// newline (the "\ No newline at end of file" shape) still round-trips byte for
// byte rather than gaining a newline the copy invented.
func TestFilePatch_NoTrailingNewline(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "nl.txt", "a\nb")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	base := headSHA(t, dir)
	writeFile(t, dir, "nl.txt", "a\nc")

	var buf bytes.Buffer
	lines, truncated, err := diff.FilePatch(context.Background(), dir, base, "nl.txt", 0, &buf)
	require.NoError(t, err)
	assert.False(t, truncated)
	assert.Equal(t, gitPatchOracle(t, dir, base, "nl.txt"), buf.String())
	assert.Equal(t, countLines(buf.String()), lines)
}

// TestFilePatch_EmptyPathIsRefused closes the trapdoor under this whole API.
// ":(top,literal)" with nothing after it is a valid pathspec that matches the
// top of the tree — every file — so an empty path silently turns the one query
// guaranteed to be O(one file) into the O(whole diff) read the windowed API
// exists to abolish. On the perf fixture that is 1.44 million lines.
func TestFilePatch_EmptyPathIsRefused(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	writeFile(t, dir, "b.txt", "two\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")
	base := headSHA(t, dir)
	writeFile(t, dir, "a.txt", "one\nchanged\n")
	writeFile(t, dir, "b.txt", "two\nchanged\n")

	var buf bytes.Buffer
	lines, truncated, err := diff.FilePatch(context.Background(), dir, base, "", 0, &buf)
	require.Error(t, err, "an empty path must be refused, never expanded to the whole diff")
	assert.Equal(t, 0, lines)
	assert.False(t, truncated)
	assert.Empty(t, buf.String())
}

// TestFilePatch_BadRef surfaces git's failure rather than an empty patch: a
// caller handed a ref that no longer resolves must learn that, not conclude the
// file is unchanged.
func TestFilePatch_BadRef(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "a.txt", "hello\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "initial")

	var buf bytes.Buffer
	_, _, err := diff.FilePatch(context.Background(), dir, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "a.txt", 0, &buf)
	require.Error(t, err)
}

// --- helpers ---

func firstLine(
	s string,
) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func countLines(
	s string,
) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

func numberedLines(
	from int,
	to int,
) string {
	var sb strings.Builder
	for i := from; i <= to; i++ {
		sb.WriteString("line ")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// editEvery rewrites every nth line so the resulting diff has many small hunks
// separated by context, which is the shape a hunk-boundary cut needs.
func editEvery(
	text string,
	n int,
) string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i := range lines {
		if i%n != 0 {
			continue
		}
		lines[i] = "EDITED " + lines[i]
	}
	return strings.Join(lines, "\n") + "\n"
}

// assertEveryHunkComplete reads each `@@` header's declared old/new line counts
// and checks the body that follows actually delivers them. This is the property
// that makes a truncated patch parseable at all.
func assertEveryHunkComplete(
	t *testing.T,
	patch string,
) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
	i := 0
	for i < len(lines) {
		if !strings.HasPrefix(lines[i], "@@ ") {
			i++
			continue
		}
		oldCount, newCount := parseHunkCounts(t, lines[i])
		i++
		gotOld, gotNew := 0, 0
		for i < len(lines) && !strings.HasPrefix(lines[i], "@@ ") && !strings.HasPrefix(lines[i], "diff --git ") {
			gotOld, gotNew = advanceCounts(lines[i], gotOld, gotNew)
			i++
		}
		assert.Equal(t, oldCount, gotOld, "hunk body is short on old-side lines; a parser would read into the next hunk")
		assert.Equal(t, newCount, gotNew, "hunk body is short on new-side lines; a parser would read into the next hunk")
	}
}

func advanceCounts(
	line string,
	gotOld int,
	gotNew int,
) (int, int) {
	if line == "" {
		return gotOld + 1, gotNew + 1
	}
	switch line[0] {
	case '+':
		return gotOld, gotNew + 1
	case '-':
		return gotOld + 1, gotNew
	case ' ':
		return gotOld + 1, gotNew + 1
	default:
		return gotOld, gotNew
	}
}

func parseHunkCounts(
	t *testing.T,
	header string,
) (int, int) {
	t.Helper()
	fields := strings.Fields(header)
	require.GreaterOrEqual(t, len(fields), 3, "malformed hunk header %q", header)
	return rangeCount(t, strings.TrimPrefix(fields[1], "-")), rangeCount(t, strings.TrimPrefix(fields[2], "+"))
}

func rangeCount(
	t *testing.T,
	spec string,
) int {
	t.Helper()
	_, count, found := strings.Cut(spec, ",")
	if !found {
		return 1
	}
	n, err := strconv.Atoi(count)
	require.NoError(t, err, "malformed hunk range %q", spec)
	return n
}

// assertApplies is the strongest validity oracle available: git itself accepts
// the truncated patch against the base tree.
func assertApplies(
	t *testing.T,
	dir string,
	base string,
	patch string,
) {
	t.Helper()
	clone := initRepo(t)
	mustGit(t, clone, "remote", "add", "origin", dir)
	mustGit(t, clone, "fetch", "-q", "origin", base)
	mustGit(t, clone, "checkout", "-q", base)

	r := exec.GitWithStdin(context.Background(), clone, patch, "apply", "--check", "-")
	require.Equal(t, 0, r.ExitCode, "truncated patch is not applyable: %s\n---patch---\n%s", r.Stderr, patch)
}
