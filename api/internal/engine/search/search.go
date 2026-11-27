package search

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/search/internal/ignore"
	"github.com/char2cs/crowbar/api/internal/engine/search/internal/match"
	"github.com/char2cs/crowbar/api/internal/engine/search/internal/replace"
	"github.com/char2cs/crowbar/api/internal/engine/search/internal/walk"
)

var ErrLocked = problem.New(problem.Locked, "workspace is locked")

var ErrPathOutsideWorkspace = problem.New(problem.InvalidArgument, "path is outside the workspace")

var ErrBadPattern = problem.New(problem.InvalidArgument, "invalid search pattern")

const (
	defaultMaxResults = 1000
	binarySampleSize  = 8192
	scannerBufSize    = 512 * 1024
)

type SearchRequest struct {
	Query         string
	CaseSensitive bool
	WholeWord     bool
	Regex         bool
	Include       []string
	Exclude       []string
}

type SearchResult struct {
	FilePath   string `json:"filePath"`
	LineNumber int    `json:"lineNumber"`
	LineText   string `json:"lineText"`
	MatchStart int    `json:"matchStart"`
	MatchEnd   int    `json:"matchEnd"`
}

type SearchResponse struct {
	Results   []SearchResult `json:"results"`
	Truncated bool           `json:"truncated"`
}

type ReplaceRequest struct {
	Query         string
	Replacement   string
	Scope         string
	CaseSensitive bool
	WholeWord     bool
	Regex         bool
}

type SearchEngine interface {
	Search(
		ctx context.Context,
		repoPath string,
		req SearchRequest,
	) (SearchResponse, error)

	Replace(
		ctx context.Context,
		repoPath string,
		req ReplaceRequest,
		locked bool,
	) error
}

type searchEngine struct{}

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
		return SearchResponse{}, fmt.Errorf("%w: %w", ErrBadPattern, err)
	}

	ig := buildIgnoreStack(repoPath)
	paths, err := walk.Walk(ctx, repoPath, ig, req.Include, req.Exclude)
	if errors.Is(err, walk.ErrBadGlob) {
		return SearchResponse{}, fmt.Errorf("%w: %w", ErrBadPattern, err)
	}
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
			continue
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
		return fmt.Errorf("%w: %w", ErrBadPattern, err)
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

func buildIgnoreStack(
	repoPath string,
) *ignore.Stack {
	var s ignore.Stack
	err := filepath.WalkDir(repoPath, func(
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
		if d.Name() == ".git" {
			return filepath.SkipDir
		}
		if err := s.Add(path, filepath.Join(path, ".gitignore")); err != nil {
			slog.Warn("search: ignore file skipped", "dir", path, "err", err)
		}
		return nil
	})
	if err != nil {
		slog.Warn("search: ignore files not all loaded", "repo", repoPath, "err", err)
	}
	return &s
}

func searchFile(
	absPath string,
	repoPath string,
	re *regexp.Regexp,
) (_ []SearchResult, err error) {
	f, err := os.Open(absPath) //nolint:gosec // path comes from filepath.WalkDir under repoPath
	if err != nil {
		return nil, fmt.Errorf("search: open %s: %w", absPath, err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()

	sample := make([]byte, binarySampleSize)
	n, err := f.Read(sample)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("search: read sample %s: %w", absPath, err)
	}

	if match.IsBinary(sample[:n]) {
		return nil, nil
	}

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

func replaceTargets(
	ctx context.Context,
	repoPath string,
	req ReplaceRequest,
) ([]string, error) {
	const filePrefix = "file:"

	if strings.HasPrefix(req.Scope, filePrefix) {
		rel := strings.TrimPrefix(req.Scope, filePrefix)
		abs := filepath.Join(repoPath, rel)
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

func drainChannel(
	ch <-chan string,
) {
	go func() {
		defer safego.Recover("search.drainChannel")
		for range ch {
		}
	}()
}
