// Package log queries a git repository's commit history.
package log

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

const (
	recordSep    = "\x1e"
	defaultLimit = 50
	formatFields = "%H\n%h\n%s\n%b\n\x1f%aN\n%aE\n%aI"
)

// List runs paginated git log starting from HEAD.
// limit maps to --max-count and skip maps to --skip.
// If limit is <= 0, it defaults to 50.
func List(
	ctx context.Context,
	repoPath string,
	limit int,
	skip int,
) ([]domain.Commit, error) {
	if limit <= 0 {
		limit = defaultLimit
	}

	format := recordSep + formatFields
	r, err := exec.Git(
		ctx,
		repoPath,
		"log",
		fmt.Sprintf("--skip=%d", skip),
		fmt.Sprintf("--max-count=%d", limit),
		"--format="+format,
	)
	if err != nil {
		return nil, fmt.Errorf("log: list: %w", err)
	}

	if r.ExitCode != 0 {
		if isEmptyRepo(r.Stderr) {
			return nil, nil
		}
		return nil, fmt.Errorf("log: list: exit %d: %s", r.ExitCode, strings.TrimSpace(r.Stderr))
	}

	return parseRecords(r.Stdout)
}

func isEmptyRepo(
	stderr string,
) bool {
	return strings.Contains(stderr, "does not have any commits yet")
}

func parseRecords(
	output string,
) ([]domain.Commit, error) {
	raw := strings.TrimLeft(output, recordSep)
	if raw == "" {
		return nil, nil
	}

	records := strings.Split(raw, recordSep)
	commits := make([]domain.Commit, 0, len(records))

	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}

		c, err := parseRecord(rec)
		if err != nil {
			return nil, err
		}

		commits = append(commits, c)
	}

	return commits, nil
}

func parseRecord(
	rec string,
) (domain.Commit, error) {
	parts := strings.SplitN(rec, "\x1f", 2)
	if len(parts) != 2 {
		return domain.Commit{}, fmt.Errorf("log: malformed record: missing field separator")
	}

	bodySection := strings.TrimRight(parts[0], "\n")
	metaSection := strings.TrimLeft(parts[1], "\n")

	bodyLines := strings.Split(bodySection, "\n")
	if len(bodyLines) < 3 {
		return domain.Commit{}, fmt.Errorf("log: malformed record: too few body lines")
	}

	fullHash := bodyLines[0]
	shortHash := bodyLines[1]
	subject := bodyLines[2]

	description := ""
	if len(bodyLines) > 3 {
		description = strings.TrimSpace(strings.Join(bodyLines[3:], "\n"))
	}

	metaLines := strings.Split(metaSection, "\n")
	if len(metaLines) < 3 {
		return domain.Commit{}, fmt.Errorf("log: malformed record: too few meta lines")
	}

	author := metaLines[0]
	email := metaLines[1]
	dateStr := strings.TrimSpace(metaLines[2])

	date, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return domain.Commit{}, fmt.Errorf("log: parse date %q: %w", dateStr, err)
	}

	return domain.Commit{
		Hash:        fullHash,
		ShortHash:   shortHash,
		Message:     subject,
		Description: description,
		Author:      author,
		Email:       email,
		Date:        date,
	}, nil
}
