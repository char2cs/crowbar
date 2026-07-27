package diff

import (
	"bufio"
	"context"
	"errors"
	"io"
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
// positive maxLines caps the output, and the cut always falls on a hunk
// boundary: a hunk that does not fit whole within the cap is omitted entirely
// rather than emitted half. A client parser fed a hunk header promising more
// lines than follow produces garbage rather than an error, so a mid-hunk cut
// would corrupt the render instead of failing it. That rounding-down is also
// what bounds memory here — at most maxLines lines are ever held, and they are
// lines the caller has already asked to receive.
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
	c.hunk = nil
	c.truncated = true
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
