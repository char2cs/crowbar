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

const searchReadBufferBytes = 64 << 10

const searchPreviewBytes = 200

const searchDevNull = "/dev/null"

var (
	searchFileHeader = []byte("diff --git ")
	searchHunkHeader = []byte("@@ ")
	searchOldHeader  = []byte("--- ")
	searchNewHeader  = []byte("+++ ")
)

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

func searchArgs(
	ref string,
) []string {
	return []string{
		"-c", "core.quotePath=false",
		"diff", "-M", "--no-prefix", "-U3", ref, "--",
	}
}

func searchExitError(
	truncated bool,
	wait func() error,
) error {
	if truncated {
		return nil
	}
	return wait()
}

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

func searchUnquotePath(
	quoted string,
) string {
	unquoted, err := strconv.Unquote(quoted)
	if err != nil {
		return quoted
	}
	return unquoted
}

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
