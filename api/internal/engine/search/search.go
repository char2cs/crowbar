// Package search is the global search/replace engine (wave 2).
//
// It exposes a SearchEngine that concurrently walks a repository, applies
// gitignore filtering, and returns per-line match results with UTF-16 column
// offsets suitable for Monaco/LSP clients.
package search

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/search/internal/ignore"
	"github.com/char2cs/crowbar/api/internal/engine/search/internal/match"
	"github.com/char2cs/crowbar/api/internal/engine/search/internal/replace"
	"github.com/char2cs/crowbar/api/internal/engine/search/internal/walk"
)

// ErrLocked is returned by Replace when the workspace is locked.
var ErrLocked = errors.New("workspace is locked")

// ErrPathOutsideWorkspace is returned by Replace when the file scope resolves
// to a path outside the repository root.
var ErrPathOutsideWorkspace = errors.New("path is outside the workspace")

// ErrBadPattern is returned by Replace (and Search) when the query cannot be
// compiled into a valid regular expression.
var ErrBadPattern = errors.New("invalid search pattern")

const (
	defaultMaxResults = 1000
	binarySampleSize  = 8192
	scannerBufSize    = 512 * 1024
)

// SearchRequest holds all search parameters.
type SearchRequest struct {
	Query         string
	CaseSensitive bool
	WholeWord     bool
	Regex         bool
	Include       []string // doublestar globs (keep matches)
	Exclude       []string // doublestar globs (drop matches)
}

// SearchResult is one match within a file.
type SearchResult struct {
	FilePath   string `json:"filePath"`
	LineNumber int    `json:"lineNumber"`
	LineText   string `json:"lineText"`
	MatchStart int    `json:"matchStart"` // UTF-16 column offset
	MatchEnd   int    `json:"matchEnd"`   // UTF-16 column offset
}

// SearchResponse is the full response including truncation flag.
type SearchResponse struct {
	Results   []SearchResult `json:"results"`
	Truncated bool           `json:"truncated"`
}

// ReplaceRequest holds replace parameters.
type ReplaceRequest struct {
	Query         string
	Replacement   string
	Scope         string // "file:<path>" or "all"
	CaseSensitive bool
	WholeWord     bool
	Regex         bool
}

// SearchEngine is the global search/replace surface.
type SearchEngine interface {
	// Search searches repoPath for the query specified in req.
	// Results are capped at 1000; Truncated is set when the cap is hit.
	Search(
		ctx context.Context,
		repoPath string,
		req SearchRequest,
	) (SearchResponse, error)

	// Replace rewrites matching occurrences on disk.
	// Returns replace.ErrLocked if the workspace is provider-protected.
	Replace(
		ctx context.Context,
		repoPath string,
		req ReplaceRequest,
		locked bool,
	) error
}

type searchEngine struct{}

// New returns a new SearchEngine.
func New() SearchEngine {
	return &searchEngine{}
}

var _ SearchEngine = (*searchEngine)(nil)

func (e *searchEngine) Search(
	ctx context.Context,
	repoPath string,
	req SearchRequest,
) (SearchResponse, error) {
	re, err := match.BuildPattern(req.Query, req.CaseSensitive, req.WholeWord, req.Regex)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("%w: %s", ErrBadPattern, err)
	}

	ig := buildIgnoreStack(repoPath)
	paths, err := walk.Walk(ctx, repoPath, ig, req.Include, req.Exclude)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("search: walk: %w", err)
	}

	var results []SearchResult
	truncated := false

	for absPath := range paths {
		if len(results) >= defaultMaxResults {
			truncated = true
			drainChannel(paths)
			break
		}

		fileResults, err := searchFile(absPath, repoPath, re)
		if err != nil {
			continue // skip unreadable files
		}

		remaining := defaultMaxResults - len(results)
		if len(fileResults) > remaining {
			results = append(results, fileResults[:remaining]...)
			truncated = true
			drainChannel(paths)
			break
		}

		results = append(results, fileResults...)
	}

	return SearchResponse{
		Results:   results,
		Truncated: truncated,
	}, nil
}

func (e *searchEngine) Replace(
	ctx context.Context,
	repoPath string,
	req ReplaceRequest,
	locked bool,
) error {
	re, err := match.BuildPattern(req.Query, req.CaseSensitive, req.WholeWord, req.Regex)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrBadPattern, err)
	}

	targets, err := replaceTargets(ctx, repoPath, req)
	if err != nil {
		return err
	}

	for _, absPath := range targets {
		if err := replace.ApplyToFile(absPath, re, req.Replacement, locked); err != nil {
			if errors.Is(err, replace.ErrLocked) {
				return ErrLocked
			}
			return fmt.Errorf("search: replace %s: %w", absPath, err)
		}
	}

	return nil
}

// buildIgnoreStack loads all .gitignore files found under repoPath into a
// hierarchical Stack. Each directory is visited in order so parent rules are
// appended before child rules, matching git's precedence semantics.
func buildIgnoreStack(
	repoPath string,
) *ignore.Stack {
	var s ignore.Stack
	_ = filepath.WalkDir(repoPath, func(
		path string,
		d fs.DirEntry,
		err error,
	) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		// Always skip .git/ so we don't parse its internal gitignore-like files.
		if d.Name() == ".git" {
			return filepath.SkipDir
		}
		_ = s.Add(path, filepath.Join(path, ".gitignore"))
		return nil
	})
	return &s
}

// searchFile scans a single file for matches of re and returns SearchResult entries.
func searchFile(
	absPath string,
	repoPath string,
	re *regexp.Regexp,
) ([]SearchResult, error) {
	f, err := os.Open(absPath) //nolint:gosec // path comes from filepath.WalkDir under repoPath
	if err != nil {
		return nil, fmt.Errorf("search: open %s: %w", absPath, err)
	}
	defer func() { _ = f.Close() }()

	// Binary sniff on the first 8 KB.
	sample := make([]byte, binarySampleSize)
	n, err := f.Read(sample)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("search: read sample %s: %w", absPath, err)
	}

	if match.IsBinary(sample[:n]) {
		return nil, nil
	}

	// Seek back to the start for line-by-line scanning.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("search: seek %s: %w", absPath, err)
	}

	relPath, err := filepath.Rel(repoPath, absPath)
	if err != nil {
		relPath = absPath
	}
	relPath = filepath.ToSlash(relPath)

	var results []SearchResult
	lineNum := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, scannerBufSize), scannerBufSize)

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		spans := match.FindMatches(re, line)
		for _, sp := range spans {
			results = append(results, SearchResult{
				FilePath:   relPath,
				LineNumber: lineNum,
				LineText:   line,
				MatchStart: sp.Start,
				MatchEnd:   sp.End,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("search: scan %s: %w", absPath, err)
	}

	return results, nil
}

// replaceTargets resolves the list of absolute file paths to rewrite.
func replaceTargets(
	ctx context.Context,
	repoPath string,
	req ReplaceRequest,
) ([]string, error) {
	const filePrefix = "file:"

	if strings.HasPrefix(req.Scope, filePrefix) {
		rel := strings.TrimPrefix(req.Scope, filePrefix)
		abs := filepath.Join(repoPath, rel)
		// Guard against path traversal: ensure abs stays under repoPath.
		if !strings.HasPrefix(abs, filepath.Clean(repoPath)+string(os.PathSeparator)) {
			return nil, fmt.Errorf("%w: %s", ErrPathOutsideWorkspace, rel)
		}
		return []string{abs}, nil
	}

	ig := buildIgnoreStack(repoPath)
	paths, err := walk.Walk(ctx, repoPath, ig, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("search: walk: %w", err)
	}

	var result []string
	for p := range paths {
		result = append(result, p)
	}

	return result, nil
}

// drainChannel consumes all remaining values from ch in a goroutine to unblock
// the walker's send side.
func drainChannel(
	ch <-chan string,
) {
	go func() {
		for range ch {
		}
	}()
}
