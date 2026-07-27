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

// MaxOutlineHunksPerFile bounds what one file may contribute to an outline. A
// diff whose hunk count approaches its line count — a file rewritten line by
// line — would otherwise make the outline as large as the thing it exists to
// avoid materialising. Past the cap the leading hunks are kept and the entry is
// marked partial.
const MaxOutlineHunksPerFile = 1000

// outlineBufferBytes sizes the read buffer. It also bounds what a single line
// can cost: a line longer than this is dispatched on its leading bytes and the
// remainder is discarded unread, so a 650 KB minified line costs the buffer, not
// its length.
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

// Outline returns the hunk geometry of the working tree's diff against ref —
// the same file set as FileSummaries and DiffAgainstRef — with no diff content.
// Every file carries its `@@` shapes, which is all the client needs to reserve
// the right scroll space per file before fetching a single patch.
//
// The diff is streamed and discarded a line at a time: memory is O(hunks) plus
// one read buffer, never O(diff size), which is what lets a 46 MB patch be
// described without ever being held. -U3 matches the context the patch endpoint
// serves, so the shapes describe what the client will actually render.
func Outline(
	ctx context.Context,
	repoPath string,
	ref string,
) ([]gitdomain.FileOutline, error) {
	reader, wait, err := exec.GitStream(ctx, repoPath, "diff", "-M", "-U3", ref, "--")
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	s := &outlineScan{}
	if scanErr := s.run(bufio.NewReaderSize(reader, outlineBufferBytes)); scanErr != nil {
		return nil, scanErr
	}
	if waitErr := wait(); waitErr != nil {
		return nil, waitErr
	}
	return s.files, nil
}

// outlineScan walks the patch as a state machine rather than by prefix alone,
// because a diff can contain a diff: an added line reading `+diff --git a/x b/y`
// is content, not a file header. Tracking how many lines each hunk still owes
// tells the two apart with certainty.
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

// body consumes one line of a hunk against the counts its header promised. An
// empty line is a context line git wrote without its leading space; a `\` line
// ("No newline at end of file") belongs to no side. Anything else means the
// header lied about its size, so the hunk is closed and the line re-read as a
// header rather than letting the miscount swallow the rest of the file.
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

// newSidePath prefers the `+++` line's path over the one recovered from the
// `diff --git` header: that header names both sides on one line and so cannot be
// split unambiguously when a path contains a space, while `+++` runs to the end
// of the line. A deletion's `+++ /dev/null` names no file and is ignored.
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

// readOutlineLine returns the next line without its newline. The slice is only
// valid until the next read — nothing here retains it. A line too long for the
// buffer comes back truncated to its leading bytes, which carry every prefix
// this scanner dispatches on, and the tail is discarded without being held.
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

// diffGitPath recovers the b-side path from `a/<old> b/<new>`. The two sides
// share one line with no escaping between them, so a path containing " b/" is
// genuinely ambiguous here; the last occurrence wins and the `+++` line
// overrides the guess for every file that has one. Only binary and
// mode-change-only entries, which git gives no `+++` line, rely on this.
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
