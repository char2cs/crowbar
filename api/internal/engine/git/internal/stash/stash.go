// Package stash provides git stash operations for a repository.
package stash

import (
	"context"
	"fmt"
	"strings"
	"time"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

const stashDateFormat = "2006-01-02 15:04:05 -0700"

// gitRunner is the function used to run git commands; overridable in tests.
var gitRunner = exec.Git

// List returns all stash entries for the repository at repoPath.
func List(
	ctx context.Context,
	repoPath string,
) ([]gitdomain.Stash, error) {
	r := gitRunner(ctx, repoPath, "stash", "list", "--format=%gd%x00%s%x00%ci%x00")
	if err := exec.RequireSuccess("stash: list", r); err != nil {
		return nil, fmt.Errorf("stash: list: %w", err)
	}
	raw := strings.TrimRight(r.Stdout, "\n")
	if raw == "" {
		return nil, nil
	}
	lines := strings.Split(raw, "\n")
	stashes := make([]gitdomain.Stash, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		s, ok := parseLine(line)
		if !ok {
			continue
		}
		count, countErr := countChangedFiles(ctx, repoPath, s.ID)
		if countErr == nil {
			s.FilesChanged = count
		}
		stashes = append(stashes, s)
	}
	return stashes, nil
}

// Push stashes current working-tree changes. message is optional.
func Push(
	ctx context.Context,
	repoPath string,
	message string,
) error {
	args := []string{"stash", "push"}
	if message != "" {
		args = append(args, "-m", message)
	}
	r := gitRunner(ctx, repoPath, args...)
	return exec.RequireSuccess("stash: push", r)
}

// Apply applies a stash entry without removing it. id is "stash@{N}".
func Apply(
	ctx context.Context,
	repoPath string,
	id string,
) error {
	r := gitRunner(ctx, repoPath, "stash", "apply", id)
	return exec.RequireSuccess("stash: apply", r)
}

// Pop applies a stash entry and removes it from the stash list. id is "stash@{N}".
func Pop(
	ctx context.Context,
	repoPath string,
	id string,
) error {
	r := gitRunner(ctx, repoPath, "stash", "pop", id)
	return exec.RequireSuccess("stash: pop", r)
}

// Drop removes a stash entry without applying it. id is "stash@{N}".
func Drop(
	ctx context.Context,
	repoPath string,
	id string,
) error {
	r := gitRunner(ctx, repoPath, "stash", "drop", id)
	return exec.RequireSuccess("stash: drop", r)
}

func parseLine(
	line string,
) (gitdomain.Stash, bool) {
	parts := strings.SplitN(line, "\x00", 4)
	if len(parts) < 3 {
		return gitdomain.Stash{}, false
	}
	id := strings.TrimSpace(parts[0])
	message := strings.TrimSpace(parts[1])
	dateStr := strings.TrimSpace(parts[2])

	if id == "" {
		return gitdomain.Stash{}, false
	}
	t, err := time.Parse(stashDateFormat, dateStr)
	if err != nil {
		return gitdomain.Stash{}, false
	}
	return gitdomain.Stash{
		ID:      id,
		Message: message,
		Date:    t,
	}, true
}

func countChangedFiles(
	ctx context.Context,
	repoPath string,
	id string,
) (int, error) {
	r := gitRunner(ctx, repoPath, "stash", "show", "--stat", id)
	if err := exec.RequireSuccess("stash: count files", r); err != nil {
		return 0, fmt.Errorf("stash: count files: %w", err)
	}
	lines := strings.Split(strings.TrimRight(r.Stdout, "\n"), "\n")
	count := 0
	for _, line := range lines {
		if strings.Contains(line, "|") {
			count++
		}
	}
	return count, nil
}
