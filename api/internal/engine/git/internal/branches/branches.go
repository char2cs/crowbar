// Package branches provides git branch operations for a repository.
package branches

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

const recordSep = "---RECORD---"

// gitRunner is the function used to run git commands. It is set to exec.Git by
// default and can be overridden in tests via export_test.go.
var gitRunner = exec.Git

// List returns all branches (local and remote) for the repository at repoPath.
func List(
	ctx context.Context,
	repoPath string,
) ([]domain.Branch, error) {
	format := "%(refname:short)%0a%(HEAD)%0a%(upstream:track,nobracket)%0a%(committerdate:iso-strict)%0a%(refname)%0a" + recordSep
	r, err := gitRunner(ctx, repoPath, "branch", "-a", "--format="+format)
	if err != nil {
		return nil, fmt.Errorf("branches: list: %w", err)
	}
	if err := exec.RequireSuccess("branches: list", r); err != nil {
		return nil, err
	}
	return parseList(r.Stdout), nil
}

// Create creates a new branch. If switchTo is true the branch is checked out
// immediately. source is the starting point; empty means HEAD.
func Create(
	ctx context.Context,
	repoPath string,
	name string,
	source string,
	switchTo bool,
) error {
	if switchTo {
		args := []string{"checkout", "-b", name}
		if source != "" {
			args = append(args, source)
		}
		r, err := gitRunner(ctx, repoPath, args...)
		if err != nil {
			return fmt.Errorf("branches: create: %w", err)
		}
		return exec.RequireSuccess("branches: create", r)
	}
	args := []string{"branch", name}
	if source != "" {
		args = append(args, source)
	}
	r, err := gitRunner(ctx, repoPath, args...)
	if err != nil {
		return fmt.Errorf("branches: create: %w", err)
	}
	return exec.RequireSuccess("branches: create", r)
}

// Rename renames a branch from oldName to newName.
func Rename(
	ctx context.Context,
	repoPath string,
	oldName string,
	newName string,
) error {
	r, err := gitRunner(ctx, repoPath, "branch", "-m", oldName, newName)
	if err != nil {
		return fmt.Errorf("branches: rename: %w", err)
	}
	return exec.RequireSuccess("branches: rename", r)
}

// Delete deletes a branch using a safe delete (-d). The branch must be
// fully merged before deletion.
func Delete(
	ctx context.Context,
	repoPath string,
	name string,
) error {
	r, err := gitRunner(ctx, repoPath, "branch", "-d", name)
	if err != nil {
		return fmt.Errorf("branches: delete: %w", err)
	}
	return exec.RequireSuccess("branches: delete", r)
}

// ForceDelete deletes a branch using a force delete (-D) regardless of merge
// status. Used by workspace teardown.
func ForceDelete(
	ctx context.Context,
	repoPath string,
	name string,
) error {
	r, err := gitRunner(ctx, repoPath, "branch", "-D", name)
	if err != nil {
		return fmt.Errorf("branches: force-delete: %w", err)
	}
	return exec.RequireSuccess("branches: force-delete", r)
}

// Switch checks out an existing branch by name.
func Switch(
	ctx context.Context,
	repoPath string,
	name string,
) error {
	r, err := gitRunner(ctx, repoPath, "checkout", name)
	if err != nil {
		return fmt.Errorf("branches: switch: %w", err)
	}
	return exec.RequireSuccess("branches: switch", r)
}

func parseList(
	output string,
) []domain.Branch {
	records := strings.Split(output, recordSep+"\n")
	var branches []domain.Branch
	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		b, ok := parseRecord(rec)
		if !ok {
			continue
		}
		branches = append(branches, b)
	}
	return branches
}

func parseRecord(
	rec string,
) (domain.Branch, bool) {
	lines := strings.Split(rec, "\n")
	if len(lines) < 5 {
		return domain.Branch{}, false
	}
	refname := strings.TrimSpace(lines[0])
	if refname == "" {
		return domain.Branch{}, false
	}
	head := strings.TrimSpace(lines[1])
	track := strings.TrimSpace(lines[2])
	dateStr := strings.TrimSpace(lines[3])
	fullRef := strings.TrimSpace(lines[4])

	var b domain.Branch
	b.IsCurrent = head == "*"

	if strings.HasPrefix(fullRef, "refs/remotes/") {
		b.IsRemote = true
		b.Name = refname
	} else {
		b.Name = refname
	}

	parseTrack(track, &b)

	if dateStr != "" {
		t, err := time.Parse(time.RFC3339, dateStr)
		if err == nil {
			b.LastCommitDate = &t
		}
	}

	return b, true
}

func parseTrack(
	track string,
	b *domain.Branch,
) {
	if track == "" {
		return
	}
	if strings.Contains(track, ",") {
		parts := strings.SplitN(track, ",", 2)
		parseTrackSegment(strings.TrimSpace(parts[0]), b)
		parseTrackSegment(strings.TrimSpace(parts[1]), b)
		return
	}
	parseTrackSegment(track, b)
}

func parseTrackSegment(
	seg string,
	b *domain.Branch,
) {
	if strings.HasPrefix(seg, "ahead ") {
		n, err := strconv.Atoi(strings.TrimPrefix(seg, "ahead "))
		if err == nil {
			b.Ahead = &n
		}
		return
	}
	if strings.HasPrefix(seg, "behind ") {
		n, err := strconv.Atoi(strings.TrimPrefix(seg, "behind "))
		if err == nil {
			b.Behind = &n
		}
	}
}
