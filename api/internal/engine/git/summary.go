package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/conflicts"
)

func (e *engine) computeWorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (added, deleted int, hasConflicts, hasCommits bool, err error) {
	hasConflicts, err = conflicts.HasConflicts(ctx, repoPath)
	if err != nil {
		return 0, 0, false, false, fmt.Errorf("git: summary: conflicts: %w", err)
	}

	if forkPointSha != "" {
		added, deleted, err = e.numstatFromForkPoint(ctx, repoPath, forkPointSha)
		if err != nil {
			return 0, 0, hasConflicts, false, fmt.Errorf("git: summary: numstat: %w", err)
		}
	}

	hasCommits, err = e.revListHasCommits(ctx, repoPath, forkPointSha)
	if err != nil {
		return added, deleted, hasConflicts, false, fmt.Errorf("git: summary: revlist: %w", err)
	}

	return added, deleted, hasConflicts, hasCommits, nil
}

func (e *engine) numstatFromForkPoint(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, error) {
	r := e.exec(ctx, repoPath, "diff", "--numstat", forkPointSha)
	if r.ExitCode != 0 {
		return 0, 0, nil
	}
	return parseNumstat(r.Stdout)
}

func parseNumstat(
	output string,
) (int, int, error) {
	var totalAdded, totalDeleted int
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		a, _ := strconv.Atoi(fields[0])
		d, _ := strconv.Atoi(fields[1])
		if a < 0 {
			a = 0
		}
		if d < 0 {
			d = 0
		}
		totalAdded += a
		totalDeleted += d
	}
	return totalAdded, totalDeleted, nil
}

func (e *engine) revListHasCommits(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (bool, error) {
	ref := forkPointSha + "..HEAD"
	if forkPointSha == "" {
		ref = "HEAD"
	}
	r := e.exec(ctx, repoPath, "rev-list", "--count", ref)
	if r.ExitCode != 0 {
		return false, nil
	}
	count, _ := strconv.Atoi(strings.TrimSpace(r.Stdout))
	return count > 0, nil
}
