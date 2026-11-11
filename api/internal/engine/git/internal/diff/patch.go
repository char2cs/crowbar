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

// patchReadBufferBytes is a throughput knob only: ReadString grows past it for an
// oversized line, unlike bufio.Scanner's 64 KB token cap that minified bundles exceed.
const patchReadBufferBytes = 64 << 10

// ErrEmptyPatchPath guards a trapdoor: ":(top,literal)" with nothing appended matches the
// whole tree, turning the O(one file) query into a read of the entire branch diff.
var ErrEmptyPatchPath = errors.New("diff: file patch: path is required")

// FilePatch streams one file's patch (by NEW name, literal top-anchored pathspec) from git
// straight into w; renames read as additions, so pairing comes from the outline. A positive
// maxLines cuts inside the last hunk and rewrites its @@ header, so capped files are never empty.
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

// patchError picks a copy's verdict: closing early kills git, so on a truncated copy the
// close and wait errors describe the requested kill, not a failure.
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

// accept routes one raw line to the writer or the pending hunk; uncapped copies never
// buffer, since there is no cut to place and a hunk can be huge.
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

// cutHunk trims a buffered hunk to room lines and recounts its @@ header; any body prefix
// is valid, so only the counts need fixing. room < 2 or an unparseable header yields nothing.
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
	// A cut inside leading context leaves a hunk that changes nothing, which git rejects
	// as corrupt; drop it.
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
