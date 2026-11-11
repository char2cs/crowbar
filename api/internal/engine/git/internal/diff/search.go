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

// searchReadBufferBytes is a throughput knob, not a limit: long lines are reassembled across reads,
// which is why this avoids bufio.Scanner's 64 KB token cap (minified bundles exceed it).
const searchReadBufferBytes = 64 << 10

// searchPreviewBytes caps one hit's preview: a hit in a single-line minified bundle would otherwise
// put most of a megabyte into the response.
const searchPreviewBytes = 200

const searchDevNull = "/dev/null"

var (
	searchFileHeader = []byte("diff --git ")
	searchHunkHeader = []byte("@@ ")
	searchOldHeader  = []byte("--- ")
	searchNewHeader  = []byte("+++ ")
)

// SearchDiff streams the working-tree diff against ref and reports every hunk-CONTENT match (headers
// describe the patch, not the code), closing git once Limit fills so memory and time stay bounded.
// An empty query matches nothing, so a find box does not scan the whole diff on open.
func SearchDiff(
	ctx context.Context,
	repoPath string,
	ref string,
	query string,
	opts gitdomain.SearchOpts,
) (_ []gitdomain.SearchHit, _ bool, err error) {
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
	defer func() { err = errors.Join(err, reader.Close()) }()
	s := &diffSearch{match: match, limit: opts.Limit}
	if scanErr := s.scan(reader); scanErr != nil {
		return nil, false, scanErr
	}
	return s.hits, s.truncated, searchExitError(s.truncated, wait)
}

// searchArgs pins path extraction: --no-prefix leaves only the path on ---/+++ lines regardless of
// diff.noprefix/mnemonicPrefix, and core.quotePath=false keeps non-ASCII paths openable.
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

// searchMatcher compiles the query once. Literal search is a byte compare; everything else, even a
// case-insensitive literal, goes through RE2 rather than lowercasing every line.
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

// consume routes one line. Prefix tests come first safely: hunk-body lines always carry a one-char
// prefix, so "diff --git " or "@@ " at column zero is patch machinery.
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

// header reads the file-name lines; a deletion's /dev/null new side falls back to the old path so its
// hits stay attributable. Other header lines are inert.
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

// searchReadLine returns the next line via allocation-free ReadSlice, copying into scratch only for a
// line spanning several reads.
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

// searchHeaderPath extracts the path after "--- "/"+++ ": a name with a space is followed by a TAB
// (the old timestamp column) that must be cut, and a name with quotes/backslashes/controls is
// C-quoted whole even with core.quotePath=false.
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

// searchUnquotePath decodes git's C-quoting with Go's literal syntax (same escapes and octal); an
// undecodable field passes through, since a mangled path still names the file.
func searchUnquotePath(
	quoted string,
) string {
	unquoted, err := strconv.Unquote(quoted)
	if err != nil {
		return quoted
	}
	return unquoted
}

// searchPreview cuts on a rune boundary: invalid UTF-8 would serialise as replacement characters and
// read as file corruption.
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
