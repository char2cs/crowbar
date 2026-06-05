// Package blame parses git blame output into per-line annotations.
package blame

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

type commitMeta struct {
	author  string
	email   string
	date    time.Time
	summary string
}

// gitRunner is the function used to run git commands; overridable in tests.
var gitRunner = exec.Git

// File runs git blame --porcelain on filePath inside repoPath and returns
// one BlameEntry per line, annotated with the commit that last changed it.
func File(
	ctx context.Context,
	repoPath string,
	filePath string,
) ([]domain.BlameEntry, error) {
	r, err := gitRunner(ctx, repoPath, "blame", "--porcelain", filePath)
	if err != nil {
		return nil, fmt.Errorf("blame: file: %w", err)
	}

	if r.ExitCode != 0 {
		return nil, fmt.Errorf("blame: file: exit %d: %s", r.ExitCode, strings.TrimSpace(r.Stderr))
	}

	return parsePorcelain(r.Stdout)
}

// parseCommitHeader extracts the SHA and result line number from a porcelain
// commit-header line ("SHA origLine resultLine count"), consulting cache for
// previously seen commit metadata.
func parseCommitHeader(
	line string,
	cache map[string]commitMeta,
) (sha string, lineNo int, meta commitMeta) {
	fields := strings.Fields(line)
	sha = fields[0]
	if len(fields) >= 3 {
		n, err := strconv.Atoi(fields[2])
		if err == nil {
			lineNo = n
		}
	}
	meta = commitMeta{}
	if existing, ok := cache[sha]; ok {
		meta = existing
	}
	return sha, lineNo, meta
}

func parsePorcelain(
	output string,
) ([]domain.BlameEntry, error) {
	lines := strings.Split(output, "\n")
	cache := make(map[string]commitMeta)
	var entries []domain.BlameEntry

	var currentSHA string
	var finalLine int
	var meta commitMeta

	for _, line := range lines {
		if line == "" {
			continue
		}

		if isCommitHeader(line) {
			currentSHA, finalLine, meta = parseCommitHeader(line, cache)
			continue
		}

		if strings.HasPrefix(line, "\t") {
			if _, cached := cache[currentSHA]; !cached {
				cache[currentSHA] = meta
			}

			entries = append(entries, domain.BlameEntry{
				LineNumber:    finalLine,
				CommitHash:    currentSHA,
				Author:        meta.author,
				Email:         meta.email,
				Date:          meta.date,
				CommitMessage: meta.summary,
			})

			continue
		}

		meta = applyField(meta, line)
		cache[currentSHA] = meta
	}

	return entries, nil
}

func isCommitHeader(
	line string,
) bool {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return false
	}

	sha := fields[0]
	if len(sha) != 40 {
		return false
	}

	for _, c := range sha {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}

	return true
}

func applyField(
	m commitMeta,
	line string,
) commitMeta {
	switch {
	case strings.HasPrefix(line, "author "):
		m.author = strings.TrimPrefix(line, "author ")
	case strings.HasPrefix(line, "author-mail "):
		raw := strings.TrimPrefix(line, "author-mail ")
		m.email = strings.Trim(raw, "<>")
	case strings.HasPrefix(line, "author-time "):
		ts := strings.TrimPrefix(line, "author-time ")
		m.date = parseUnixTime(ts)
	case strings.HasPrefix(line, "summary "):
		m.summary = strings.TrimPrefix(line, "summary ")
	}

	return m
}

func parseUnixTime(
	s string,
) time.Time {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return time.Time{}
	}

	return time.Unix(n, 0).UTC()
}
