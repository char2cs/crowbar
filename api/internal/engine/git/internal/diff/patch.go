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

const patchReadBufferBytes = 64 << 10

var ErrEmptyPatchPath = errors.New("diff: file patch: path is required")

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

var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)$`)

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
		default:
			oldCount++
			newCount++
		}
	}
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
