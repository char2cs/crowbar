package git

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/conflicts"
)

func (e *engine) computeWorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	base string,
) (added, deleted int, hasConflicts, hasCommits bool, err error) {
	hasConflicts, err = conflicts.HasConflicts(ctx, repoPath)
	if err != nil {
		return 0, 0, false, false, fmt.Errorf("git: summary: conflicts: %w", err)
	}

	diffBase := e.diffBase(ctx, repoPath, base)
	added, deleted, err = e.numstatFromBase(ctx, repoPath, diffBase.Commit)
	if err != nil {
		return 0, 0, hasConflicts, false, fmt.Errorf("git: summary: numstat: %w", err)
	}
	return added, deleted, hasConflicts, diffBase.HasCommits, nil
}

// diffBase resolves base to the commit the summary diffs against and whether
// the branch has commits beyond it. With no resolvable base it falls back to
// "does the repo have any commit".
func (e *engine) diffBase(
	ctx context.Context,
	repoPath string,
	base string,
) gitdomain.DiffBase {
	commit, ahead := e.resolveDiffBase(ctx, repoPath, base)
	if commit == "" {
		return gitdomain.DiffBase{HasCommits: e.repoHasCommits(ctx, repoPath)}
	}
	return gitdomain.DiffBase{Commit: commit, HasCommits: ahead > 0}
}

// resolveDiffBase returns merge-base(base, HEAD) so the summary counts only the branch's own changes;
// a branch name tries origin/<b> and <b> and the merge-base closest to HEAD wins (local may be ahead).
// "" means no candidate resolves (no committed diff); the second result is HEAD's commits beyond it.
func (e *engine) resolveDiffBase(
	ctx context.Context,
	repoPath string,
	base string,
) (string, int) {
	if base == "" {
		return "", 0
	}
	type candidate struct {
		ref   string
		ahead int
	}
	var found []candidate
	for _, ref := range baseRefCandidates(base) {
		if ahead, ok := e.commitsAheadOf(ctx, repoPath, ref); ok {
			found = append(found, candidate{ref, ahead})
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].ahead < found[j].ahead })
	for _, c := range found {
		r := e.exec(ctx, repoPath, "merge-base", c.ref, "HEAD")
		if r.ExitCode != 0 {
			continue
		}
		if mergeBase := strings.TrimSpace(r.Stdout); mergeBase != "" {
			return mergeBase, c.ahead
		}
	}
	return "", 0
}

// baseRefCandidates: a branch name is probed as its remote-tracking and local ref; a full
// SHA is used verbatim, since origin/<sha> never resolves.
func baseRefCandidates(
	base string,
) []string {
	if looksLikeCommitSHA(base) {
		return []string{base}
	}
	return []string{"origin/" + base, base}
}

// looksLikeCommitSHA reports whether s is a full 40-character hex object name, the
// form RevParse records as a fork point. Branch names are effectively never 40
// hex chars, so this cleanly separates the SHA base from the branch-name base.
func looksLikeCommitSHA(
	s string,
) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}

// commitsAheadOf counts `rev-list ref..HEAD`: the distance from the merge-base to HEAD,
// ranking candidates without a merge-base spawn each. ok is false when ref does not resolve.
func (e *engine) commitsAheadOf(
	ctx context.Context,
	repoPath string,
	ref string,
) (int, bool) {
	r := e.exec(ctx, repoPath, "rev-list", "--count", ref+"..HEAD")
	if r.ExitCode != 0 {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(r.Stdout))
	return n, err == nil
}

func (e *engine) numstatFromBase(
	ctx context.Context,
	repoPath string,
	base string,
) (int, int, error) {
	if base == "" {
		return 0, 0, nil
	}
	r := e.exec(ctx, repoPath, "diff", "--numstat", base)
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
		totalAdded += numstatCount(fields[0])
		totalDeleted += numstatCount(fields[1])
	}
	return totalAdded, totalDeleted, nil
}

func (e *engine) repoHasCommits(
	ctx context.Context,
	repoPath string,
) bool {
	r := e.exec(ctx, repoPath, "rev-list", "--count", "HEAD")
	if r.ExitCode != 0 {
		return false
	}
	count, err := strconv.Atoi(strings.TrimSpace(r.Stdout))
	return err == nil && count > 0
}

// numstatCount is one numstat column. A binary file's columns read "-", and
// count no lines.
func numstatCount(
	field string,
) int {
	n, err := strconv.Atoi(field)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
