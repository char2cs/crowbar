package diff

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// searchReadBufferBytes sizes the buffer the scanner pulls git's stdout
// through. It is a throughput knob, not a limit: a line longer than the buffer
// is reassembled across reads rather than failing, which is why this path does
// not use a bufio.Scanner — Scanner caps a token at 64 KB by default and real
// repositories carry minified bundles with lines several hundred KB long.
const searchReadBufferBytes = 64 << 10

// searchPreviewBytes caps one hit's preview. The perf fixture's minified bundle
// is a single 657,780-character line; without a cap, one hit in it would put
// two thirds of a megabyte into the response, and a query matching it in every
// file would put the whole diff there — exactly what streaming avoided.
const searchPreviewBytes = 200

const searchDevNull = "/dev/null"

var (
	searchFileHeader = []byte("diff --git ")
	searchHunkHeader = []byte("@@ ")
	searchOldHeader  = []byte("--- ")
	searchNewHeader  = []byte("+++ ")
)

// SearchDiff scans the working-tree diff against ref — the same blend of
// committed and uncommitted changes AgainstRef returns — for query, and reports
// the file and line number of every match, plus whether the Limit cut the
// results short.
//
// The diff is streamed and matched a line at a time, so a search over a 46 MB
// patch costs the memory of one line plus the hits kept, never the patch. When
// Limit fills, the reader is closed and git is killed where it stands, so a
// query that matches early does not pay for the rest of the diff.
//
// Only hunk CONTENT is searched. A query matching a `diff --git` line, a `@@`
// header (including the function name git appends to it) or a `---`/`+++` file
// header is not a hit — those describe the patch rather than the code, and
// sending a reader to them is worse than finding nothing.
//
// An empty query matches nothing rather than everything, so a find box that
// searches on every keystroke does not run a whole-diff scan the moment it
// opens.
func SearchDiff(
	ctx context.Context,
	repoPath string,
	ref string,
	query string,
	opts gitdomain.SearchOpts,
) ([]gitdomain.SearchHit, bool, error) {
	if query == "" {
		return nil, false, nil
	}
	match, err := searchMatcher(query, opts)
	if err != nil {
		return nil, false, err
	}
	reader, wait, err := exec.GitStream(ctx, repoPath, searchArgs(ref)...)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = reader.Close() }()
	s := &diffSearch{match: match, limit: opts.Limit}
	if scanErr := s.scan(reader); scanErr != nil {
		return nil, false, scanErr
	}
	return s.hits, s.truncated, searchExitError(s.truncated, wait)
}

// searchArgs pins the two things path extraction depends on. --no-prefix drops
// the a/ and b/ decorations, so the `---`/`+++` lines carry the path and
// nothing else — no guessing whether a leading "b/" belongs to the patch or to
// a directory actually named b, and no exposure to diff.noprefix or
// diff.mnemonicPrefix rewriting the decoration out from under the parser.
// core.quotePath=false stops git C-quoting non-ASCII paths, which would
// otherwise reach the client as "src/w\303\244rme.txt" — not a path it can open.
func searchArgs(
	ref string,
) []string {
	return []string{
		"-c", "core.quotePath=false",
		"diff", "-M", "--no-prefix", "-U3", ref, "--",
	}
}

// searchExitError suppresses git's verdict on a truncated search. Closing the
// reader early kills the subprocess, so wait then describes the kill this
// search asked for rather than a failure the caller should hear about.
func searchExitError(
	truncated bool,
	wait func() error,
) error {
	if truncated {
		return nil
	}
	return wait()
}

// searchMatcher compiles the query once, outside the scan. A literal search is
// a byte comparison; every other mode goes through RE2, including the
// case-insensitive literal — quoting the query and letting the engine fold case
// avoids lowercasing a million lines to search them.
func searchMatcher(
	query string,
	opts gitdomain.SearchOpts,
) (func([]byte) bool, error) {
	if !opts.Regex && opts.CaseSensitive {
		needle := []byte(query)
		return func(line []byte) bool { return bytes.Contains(line, needle) }, nil
	}
	pattern := query
	if !opts.Regex {
		pattern = regexp.QuoteMeta(query)
	}
	if !opts.CaseSensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("diff: search pattern %q: %w", query, err)
	}
	return re.Match, nil
}

// diffSearch is the unified-diff state machine. It holds one line at a time and
// the hits found so far; nothing it keeps grows with the size of the diff.
type diffSearch struct {
	match func([]byte) bool
	limit int

	path    string
	oldPath string
	inHunk  bool
	oldLine int
	newLine int

	hits      []gitdomain.SearchHit
	truncated bool
}

func (s *diffSearch) scan(
	src io.Reader,
) error {
	r := bufio.NewReaderSize(src, searchReadBufferBytes)
	var scratch []byte
	for !s.truncated {
		line, next, err := searchReadLine(r, scratch)
		scratch = next
		if err != nil {
			return s.finish(line, err)
		}
		s.consume(line)
	}
	return nil
}

// finish handles the last read, which carries both the final line — a file
// whose last line has no trailing newline produces one — and the reason the
// stream ended.
func (s *diffSearch) finish(
	line []byte,
	err error,
) error {
	if len(line) > 0 {
		s.consume(line)
	}
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// consume routes one line. The prefix tests run before any content test and are
// safe in that order: every line of a hunk body carries a one-character prefix,
// so a line beginning "diff --git " or "@@ " at column zero can only be the
// patch's own machinery, never a line of the file.
func (s *diffSearch) consume(
	line []byte,
) {
	line = searchTrimEOL(line)
	if bytes.HasPrefix(line, searchFileHeader) {
		s.startFile()
		return
	}
	if bytes.HasPrefix(line, searchHunkHeader) {
		s.startHunk(line)
		return
	}
	if !s.inHunk {
		s.header(line)
		return
	}
	s.body(line)
}

func (s *diffSearch) startFile() {
	s.inHunk = false
	s.path = ""
	s.oldPath = ""
}

// header reads the two file-name lines. A deletion's new side is /dev/null, so
// the path falls back to the old one — otherwise every hit in a deleted file
// would be unattributable. Every other header line (index, mode, similarity,
// "Binary files … differ") is inert.
func (s *diffSearch) header(
	line []byte,
) {
	if bytes.HasPrefix(line, searchOldHeader) {
		s.oldPath = searchHeaderPath(line[len(searchOldHeader):])
		return
	}
	if !bytes.HasPrefix(line, searchNewHeader) {
		return
	}
	s.path = searchHeaderPath(line[len(searchNewHeader):])
	if s.path == "" {
		s.path = s.oldPath
	}
}

// startHunk resets both counters to what the header declares. Restarting from
// the header on every hunk — rather than counting on from the previous one — is
// what keeps the numbers right across the gaps a diff skips.
func (s *diffSearch) startHunk(
	line []byte,
) {
	oldStart, newStart, ok := searchHunkStarts(line)
	if !ok {
		return
	}
	s.inHunk = true
	s.oldLine = oldStart
	s.newLine = newStart
}

// body advances the counters by the line's prefix and matches its content. A
// "\" line ("\ No newline at end of file") annotates the previous line and
// advances neither side.
func (s *diffSearch) body(
	line []byte,
) {
	if len(line) == 0 {
		s.oldLine++
		s.newLine++
		return
	}
	switch line[0] {
	case '+':
		s.record(line[1:], gitdomain.SearchSideNew, s.newLine)
		s.newLine++
	case '-':
		s.record(line[1:], gitdomain.SearchSideOld, s.oldLine)
		s.oldLine++
	case ' ':
		s.record(line[1:], gitdomain.SearchSideNew, s.newLine)
		s.oldLine++
		s.newLine++
	}
}

func (s *diffSearch) record(
	content []byte,
	side string,
	number int,
) {
	if !s.match(content) {
		return
	}
	s.hits = append(s.hits, gitdomain.SearchHit{
		Path:       s.path,
		Side:       side,
		LineNumber: number,
		Preview:    searchPreview(content),
	})
	s.truncated = s.limit > 0 && len(s.hits) >= s.limit
}

// searchReadLine returns the next line, newline included, reusing scratch for
// the rare line too long for the reader's buffer. ReadSlice hands back a view
// into that buffer and costs no allocation, which matters at a million lines;
// the copy into scratch only happens for a line that spans several reads.
func searchReadLine(
	r *bufio.Reader,
	scratch []byte,
) ([]byte, []byte, error) {
	line, err := r.ReadSlice('\n')
	if !errors.Is(err, bufio.ErrBufferFull) {
		return line, scratch, err
	}
	scratch = append(scratch[:0], line...)
	for errors.Is(err, bufio.ErrBufferFull) {
		line, err = r.ReadSlice('\n')
		scratch = append(scratch, line...)
	}
	return scratch, scratch, err
}

func searchTrimEOL(
	line []byte,
) []byte {
	line = bytes.TrimSuffix(line, []byte("\n"))
	return bytes.TrimSuffix(line, []byte("\r"))
}

// searchHunkStarts parses the two starts out of `@@ -a,b +c,d @@ context`. A
// count omitted from either side means one line, and neither count is needed
// here — the prefixes of the body lines say where the hunk ends.
func searchHunkStarts(
	line []byte,
) (int, int, bool) {
	fields := strings.Fields(string(line))
	if len(fields) < 3 {
		return 0, 0, false
	}
	if !strings.HasPrefix(fields[1], "-") || !strings.HasPrefix(fields[2], "+") {
		return 0, 0, false
	}
	oldStart, oldOK := searchRangeStart(fields[1][1:])
	newStart, newOK := searchRangeStart(fields[2][1:])
	return oldStart, newStart, oldOK && newOK
}

func searchRangeStart(
	field string,
) (int, bool) {
	start, _, _ := strings.Cut(field, ",")
	n, err := strconv.Atoi(start)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// searchHeaderPath extracts the path from what follows "--- " or "+++ ". Two
// spellings reach it. A name carrying a space is followed by a TAB, the field
// separator the unified-diff format inherits from POSIX diff's optional
// timestamp column — leave it on and every such path is one the client cannot
// open. A name carrying a quote, a backslash or a control byte is C-quoted as a
// whole, which core.quotePath=false does not turn off: that setting governs
// non-ASCII bytes only.
func searchHeaderPath(
	field []byte,
) string {
	path := string(field)
	if strings.HasPrefix(path, `"`) {
		return searchUnquotePath(path)
	}
	name, _, _ := strings.Cut(path, "\t")
	if name == searchDevNull {
		return ""
	}
	return name
}

// searchUnquotePath decodes git's C-quoting, which Go's own string-literal
// syntax covers: the same \" \\ \t escapes and the same three-digit octal form
// git uses for a raw byte. A field that will not decode is passed through
// rather than dropped — a mangled path still says which file to look in.
func searchUnquotePath(
	quoted string,
) string {
	unquoted, err := strconv.Unquote(quoted)
	if err != nil {
		return quoted
	}
	return unquoted
}

// searchPreview cuts the content to the cap on a rune boundary. Cutting mid
// rune would put invalid UTF-8 into a JSON response, where it serialises as a
// replacement character and reads as corruption in the file rather than in the
// preview.
func searchPreview(
	content []byte,
) string {
	if len(content) <= searchPreviewBytes {
		return string(content)
	}
	cut := searchPreviewBytes
	for cut > 0 && !utf8.RuneStart(content[cut]) {
		cut--
	}
	return string(content[:cut])
}
