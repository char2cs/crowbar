package diff

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// MaxOutlineHunksPerFile bounds one file's outline: a line-by-line rewrite would otherwise make it
// as large as the diff. Past the cap the leading hunks are kept and the entry is marked partial.
const MaxOutlineHunksPerFile = 1000

// outlineBufferBytes also bounds a single line's cost: a longer line is dispatched on its
// leading bytes and the rest discarded unread.
const outlineBufferBytes = 64 << 10

var (
	diffGitPrefix    = []byte("diff --git ")
	renameFromPrefix = []byte("rename from ")
	renameToPrefix   = []byte("rename to ")
	newSidePrefix    = []byte("+++ ")
	binaryPrefix     = []byte("Binary files ")
	hunkPrefix       = []byte("@@ ")
)

const devNull = "/dev/null"

// Outline returns the hunk geometry (no content) of FileSummaries' file set so the client can reserve
// scroll space; streamed a line at a time, O(hunks) memory. -U3 matches the patch endpoint.
func Outline(
	ctx context.Context,
	repoPath string,
	ref string,
) (_ []gitdomain.FileOutline, err error) {
	reader, wait, err := exec.GitStream(ctx, repoPath, "diff", "-M", "-U3", ref, "--")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, reader.Close()) }()

	s := &outlineScan{}
	if scanErr := s.run(bufio.NewReaderSize(reader, outlineBufferBytes)); scanErr != nil {
		return nil, scanErr
	}
	if waitErr := wait(); waitErr != nil {
		return nil, waitErr
	}
	return s.files, nil
}

// outlineScan is a state machine, not a prefix match: a diff can contain a diff, and the
// lines each hunk still owes tell a `+diff --git` content line from a header.
type outlineScan struct {
	files        []gitdomain.FileOutline
	current      gitdomain.FileOutline
	open         bool
	hunksSeen    int
	oldRemaining int
	newRemaining int
}

func (s *outlineScan) run(
	r *bufio.Reader,
) error {
	for {
		done, err := s.step(r)
		if err != nil {
			return err
		}
		if done {
			s.flush()
			return nil
		}
	}
}

func (s *outlineScan) step(
	r *bufio.Reader,
) (bool, error) {
	line, err := readOutlineLine(r)
	done := errors.Is(err, io.EOF)
	if done && len(line) == 0 {
		return true, nil
	}
	s.line(line)
	if done {
		return true, nil
	}
	return false, err
}

func (s *outlineScan) line(
	line []byte,
) {
	if s.inHunk() {
		s.body(line)
		return
	}
	if len(line) == 0 {
		return
	}
	s.header(line)
}

func (s *outlineScan) inHunk() bool {
	return s.oldRemaining > 0 || s.newRemaining > 0
}

// body consumes one hunk line; an empty line is unspaced context, `\` belongs to no side, and
// anything else means the header lied, so the hunk closes and the line is re-read as a header.
func (s *outlineScan) body(
	line []byte,
) {
	if len(line) == 0 {
		s.oldRemaining--
		s.newRemaining--
		return
	}
	switch line[0] {
	case '+':
		s.newRemaining--
	case '-':
		s.oldRemaining--
	case ' ':
		s.oldRemaining--
		s.newRemaining--
	case '\\':
	default:
		s.oldRemaining, s.newRemaining = 0, 0
		s.header(line)
	}
}

func (s *outlineScan) header(
	line []byte,
) {
	if bytes.HasPrefix(line, diffGitPrefix) {
		s.startFile(line)
		return
	}
	if !s.open {
		return
	}
	s.fileHeader(line)
}

func (s *outlineScan) fileHeader(
	line []byte,
) {
	switch {
	case bytes.HasPrefix(line, hunkPrefix):
		s.startHunk(line)
	case bytes.HasPrefix(line, renameFromPrefix):
		s.current.OldPath = unquotePath(string(line[len(renameFromPrefix):]))
	case bytes.HasPrefix(line, renameToPrefix):
		s.current.Path = unquotePath(string(line[len(renameToPrefix):]))
	case bytes.HasPrefix(line, newSidePrefix):
		s.newSidePath(string(line[len(newSidePrefix):]))
	case bytes.HasPrefix(line, binaryPrefix):
		s.current.IsBinary = true
	}
}

func (s *outlineScan) startFile(
	line []byte,
) {
	s.flush()
	s.current = gitdomain.FileOutline{
		Path:  diffGitPath(string(line[len(diffGitPrefix):])),
		Hunks: []gitdomain.HunkShape{},
	}
	s.open = true
	s.hunksSeen = 0
}

// newSidePath prefers `+++`'s path, which runs to end of line, over the ambiguous `diff --git`
// header; a deletion's `+++ /dev/null` is ignored.
func (s *outlineScan) newSidePath(
	raw string,
) {
	if raw == devNull {
		return
	}
	s.current.Path = trimSidePrefix(unquotePath(raw))
}

func (s *outlineScan) startHunk(
	line []byte,
) {
	shape, ok := parseHunkShape(string(line[len(hunkPrefix):]))
	if !ok {
		return
	}
	s.oldRemaining, s.newRemaining = shape.OldLines, shape.NewLines
	s.hunksSeen++
	if s.hunksSeen > MaxOutlineHunksPerFile {
		s.current.IsPartial = true
		return
	}
	s.current.Hunks = append(s.current.Hunks, shape)
}

func (s *outlineScan) flush() {
	if !s.open {
		return
	}
	s.files = append(s.files, s.current)
	s.open = false
	s.oldRemaining, s.newRemaining = 0, 0
}

// readOutlineLine returns the next line, valid until the next read; an over-long line comes
// back truncated to its leading bytes (all the scanner dispatches on).
func readOutlineLine(
	r *bufio.Reader,
) ([]byte, error) {
	line, err := r.ReadSlice('\n')
	if err == nil {
		return bytes.TrimSuffix(line, []byte("\n")), nil
	}
	if !errors.Is(err, bufio.ErrBufferFull) {
		return line, err
	}
	head := bytes.Clone(line)
	return head, discardOutlineLine(r)
}

func discardOutlineLine(
	r *bufio.Reader,
) error {
	for {
		_, err := r.ReadSlice('\n')
		if !errors.Is(err, bufio.ErrBufferFull) {
			return err
		}
	}
}

// parseHunkShape reads `-a,b +c,d @@ optional heading`, the remainder of a hunk
// header after its `@@ `. Either count may be omitted, which means one line.
func parseHunkShape(
	rest string,
) (gitdomain.HunkShape, bool) {
	ranges, _, found := strings.Cut(rest, " @@")
	if !found {
		return gitdomain.HunkShape{}, false
	}
	fields := strings.Fields(ranges)
	if len(fields) != 2 {
		return gitdomain.HunkShape{}, false
	}
	oldStart, oldLines, oldOK := parseHunkRange(fields[0], '-')
	newStart, newLines, newOK := parseHunkRange(fields[1], '+')
	if !oldOK || !newOK {
		return gitdomain.HunkShape{}, false
	}
	return gitdomain.HunkShape{
		OldStart: oldStart,
		OldLines: oldLines,
		NewStart: newStart,
		NewLines: newLines,
	}, true
}

func parseHunkRange(
	field string,
	sign byte,
) (int, int, bool) {
	if len(field) < 2 || field[0] != sign {
		return 0, 0, false
	}
	rawStart, rawCount, hasCount := strings.Cut(field[1:], ",")
	start, err := strconv.Atoi(rawStart)
	if err != nil {
		return 0, 0, false
	}
	if !hasCount {
		return start, 1, true
	}
	count, err := strconv.Atoi(rawCount)
	if err != nil {
		return 0, 0, false
	}
	return start, count, true
}

// diffGitPath recovers the b-side path; " b/" in a path is ambiguous, so the last occurrence
// wins. Only binary and mode-only entries, which have no `+++` line, rely on it.
func diffGitPath(
	rest string,
) string {
	if strings.HasSuffix(rest, `"`) {
		return trimSidePrefix(unquotePath(rest[strings.LastIndex(rest, ` "`)+1:]))
	}
	idx := strings.LastIndex(rest, " b/")
	if idx < 0 {
		return rest
	}
	return rest[idx+len(" b/"):]
}

func trimSidePrefix(
	path string,
) string {
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		return path[2:]
	}
	return path
}

// unquotePath undoes the C-style quoting git applies to a path carrying a byte
// outside printable ASCII. The escape forms git emits — octal, \t, \", \\ — are
// all valid in a Go string literal, so Unquote is the exact inverse.
func unquotePath(
	raw string,
) string {
	if !strings.HasPrefix(raw, `"`) {
		return raw
	}
	unquoted, err := strconv.Unquote(raw)
	if err != nil {
		return raw
	}
	return unquoted
}
