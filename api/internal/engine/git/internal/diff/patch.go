package diff

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// patchReadBufferBytes sizes the read buffer the patch copier pulls git's
// stdout through. It is a throughput knob only: ReadString grows past it for a
// single oversized line rather than failing, which is the whole reason this
// path does not use a bufio.Scanner — Scanner caps a token at 64 KB and real
// repositories carry minified bundles with lines several hundred KB long.
const patchReadBufferBytes = 64 << 10

// ErrEmptyPatchPath rejects a patch request that names no file. It is a guard
// against a specific trapdoor rather than mere input validation: ":(top,literal)"
// with nothing appended is a perfectly valid pathspec matching the top of the
// tree, so an empty path would quietly turn the one query guaranteed to be
// O(one file) into a read of the entire branch diff — 1.44 million lines on the
// perf fixture, which is the exact failure this API exists to abolish.
var ErrEmptyPatchPath = errors.New("diff: file patch: path is required")

// FilePatch streams one file's unified patch into w and reports how many patch
// lines it wrote and whether it stopped short of the file's full patch.
//
// It runs `git diff -M -U3 <ref> -- :(top,literal)<path>`, so the subprocess
// itself only ever produces the one file's patch and the bytes go straight from
// git's stdout to w: neither this function nor the daemon around it ever holds
// the patch. The pathspec is literal and top-anchored, so a file named ":x" or
// "we*ird[1].txt" matches itself and nothing else.
//
// path addresses the file by its NEW name. Scoping the diff to a single path is
// what keeps the query off the O(diff size) path, and it necessarily hides
// rename pairing: -M can only pair a deletion with an addition, and the
// deletion half is filtered out, so a renamed file's patch reads as an
// addition. Callers that need the pairing take it from the outline, which
// carries OldPath.
//
// A path with no changes is not an error — it yields zero lines and an empty
// patch, which is what a client asking about a file that has since been
// reverted must see. An EMPTY path is a different thing entirely and returns
// ErrEmptyPatchPath.
//
// maxLines <= 0 means unlimited, and copies with no buffering at all. A
// positive maxLines caps the output at exactly that many lines, cutting inside
// the final hunk and REWRITING that hunk's @@ header so its counts describe the
// lines actually emitted. A parser fed a header promising more lines than
// follow produces garbage rather than an error — so the header is corrected
// rather than the hunk dropped.
//
// Dropping it was the original design, and it was wrong in the one shape that
// matters most. A whole-file rewrite is a SINGLE hunk, so "omit a hunk that
// does not fit" degenerated to "emit the file header and nothing else" for
// every file bigger than the cap: the perf fixture's two 420,000-line monsters
// came back as five header lines with zero hunks. The client rendered them as
// empty — a blank region where the file should be — and, because the reserved
// height had been sized from the file's real line count, materialising one
// collapsed the scroll by 420,000 rows and threw every file below it upwards.
//
// Capping at exactly maxLines is also what lets a client reserve space
// correctly: what it reserves for a capped file is what it will be sent.
//
// At most maxLines lines are ever held, and they are lines the caller asked to
// receive, so the cap bounds this function's memory as well as its output.
//
// The file's header lines (`diff --git`, `index`, `---`/`+++`, `Binary files`)
// are always written whole, so a cap smaller than the header is exceeded by it;
// a truncated header would be no more parseable than a truncated hunk.
func FilePatch(
	ctx context.Context,
	repoPath string,
	ref string,
	path string,
	maxLines int,
	w io.Writer,
) (int, bool, error) {
	if path == "" {
		return 0, false, ErrEmptyPatchPath
	}
	reader, wait, err := exec.GitStream(
		ctx,
		repoPath,
		"diff", "-M", "-U3", ref, "--", literalPathspec+path,
	)
	if err != nil {
		return 0, false, err
	}
	c := &patchCopier{out: w, maxLines: maxLines}
	copyErr := c.run(reader)
	closeErr := reader.Close()
	return c.written, c.truncated, patchError(c.truncated, copyErr, closeErr, wait())
}

// patchError picks which of a copy's three verdicts to report. Closing the
// reader before EOF is how a truncated copy stops paying for the rest of the
// diff, and it kills git — so on that path the close and wait verdicts describe
// the kill this function asked for, not a failure to report to the caller.
func patchError(
	truncated bool,
	copyErr error,
	closeErr error,
	waitErr error,
) error {
	if copyErr != nil {
		return copyErr
	}
	if truncated {
		return nil
	}
	if closeErr != nil {
		return closeErr
	}
	return waitErr
}

type patchCopier struct {
	out      io.Writer
	maxLines int

	written   int
	hunk      []string
	truncated bool
	done      bool
	err       error
}

func (c *patchCopier) run(
	src io.Reader,
) error {
	r := bufio.NewReaderSize(src, patchReadBufferBytes)
	for !c.stopped() {
		line, readErr := r.ReadString('\n')
		c.accept(line)
		c.observe(readErr)
	}
	return c.err
}

func (c *patchCopier) stopped() bool {
	return c.done || c.truncated || c.err != nil
}

func (c *patchCopier) observe(
	readErr error,
) {
	if readErr == nil {
		return
	}
	if !errors.Is(readErr, io.EOF) {
		c.err = readErr
		return
	}
	c.flushHunk()
	c.done = true
}

// accept routes one raw line, newline included, to the writer or to the pending
// hunk buffer. Uncapped copies never buffer: with no cap there is no cut to
// place, so holding a hunk back would buy nothing and would cost the size of
// the largest hunk in the file — 420,000 lines on the perf fixture's monster.
func (c *patchCopier) accept(
	line string,
) {
	if line == "" {
		return
	}
	if c.maxLines <= 0 {
		c.write(line)
		return
	}
	if strings.HasPrefix(line, "@@") {
		c.flushHunk()
		c.buffer(line)
		return
	}
	if len(c.hunk) > 0 && !strings.HasPrefix(line, "diff --git ") {
		c.buffer(line)
		return
	}
	c.flushHunk()
	c.write(line)
}

func (c *patchCopier) buffer(
	line string,
) {
	c.hunk = append(c.hunk, line)
	if c.written+len(c.hunk) <= c.maxLines {
		return
	}
	c.hunk = cutHunk(c.hunk, c.maxLines-c.written)
	c.flushHunk()
	c.truncated = true
}

// hunkHeaderPattern matches a unified-diff hunk header, capturing the old and
// new starts, their optional counts, and the trailing section heading. Only the
// counts are rewritten; a start line and a heading survive a cut unchanged.
var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)$`)

// cutHunk trims a buffered hunk to the `room` lines that still fit and rewrites
// its @@ header so the counts match the body that survives.
//
// Any prefix of a hunk body is itself a valid body — each line is independently
// a context, addition, deletion or no-newline marker — so the only thing a cut
// invalidates is the header's promise about how many of each follow. Recounting
// the kept lines restores it.
//
// room < 2 leaves no space for a header plus even one line of body, and a
// header this function cannot parse is one it must not rewrite; both yield
// nothing rather than a hunk a client would render as garbage.
func cutHunk(
	hunk []string,
	room int,
) []string {
	if len(hunk) == 0 || room < 2 {
		return nil
	}
	if len(hunk) <= room {
		return hunk
	}
	match := hunkHeaderPattern.FindStringSubmatch(strings.TrimRight(hunk[0], "\r\n"))
	if match == nil {
		return nil
	}
	body := hunk[1:room]
	oldCount, newCount, changes := 0, 0, 0
	for _, line := range body {
		switch {
		case strings.HasPrefix(line, "+"):
			newCount++
			changes++
		case strings.HasPrefix(line, "-"):
			oldCount++
			changes++
		case strings.HasPrefix(line, `\`):
			// "\ No newline at end of file" annotates the previous line and
			// occupies no line on either side.
		default:
			// Context, including the bare newline git emits for a blank line.
			oldCount++
			newCount++
		}
	}
	// A cut can land inside a run of leading context, leaving a hunk that
	// changes nothing. Git rejects that outright ("corrupt patch"), and it would
	// show a reader a hunk header above lines identical on both sides, so it is
	// worth strictly less than the space it costs.
	if changes == 0 {
		return nil
	}
	header := fmt.Sprintf("@@ -%s,%d +%s,%d @@%s\n", match[1], oldCount, match[2], newCount, match[3])
	return append([]string{header}, body...)
}

func (c *patchCopier) flushHunk() {
	lines := c.hunk
	c.hunk = nil
	for _, line := range lines {
		c.write(line)
	}
}

func (c *patchCopier) write(
	line string,
) {
	if c.err != nil {
		return
	}
	if _, err := io.WriteString(c.out, line); err != nil {
		c.err = err
		return
	}
	c.written++
}
