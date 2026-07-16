package git

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

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

	base = e.resolveDiffBase(ctx, repoPath, base)

	if base != "" {
		added, deleted, err = e.numstatFromBase(ctx, repoPath, base)
		if err != nil {
			return 0, 0, hasConflicts, false, fmt.Errorf("git: summary: numstat: %w", err)
		}
	}

	hasCommits, err = e.revListHasCommits(ctx, repoPath, base)
	if err != nil {
		return added, deleted, hasConflicts, false, fmt.Errorf("git: summary: revlist: %w", err)
	}

	return added, deleted, hasConflicts, hasCommits, nil
}

// resolveDiffBase returns the commit a working-tree summary should diff against,
// given a base ref that is EITHER a branch name (e.g. "develop") or a fork-point
// SHA. It returns the merge-base of the base branch's freshest tip and HEAD, so
// the summary counts only the branch's OWN changes and never the base branch's
// advancement — self-correcting as the branch is rebased onto newer base commits,
// unlike a frozen fork-point SHA diffed against directly (which counts every
// commit the base branch gained since the branch was created).
//
// A branch name is resolved against BOTH origin/<branch> and the local <branch>,
// and the merge-base CLOSEST to HEAD (fewest commits between it and HEAD) wins.
// That is correct whichever ref the branch was actually built on: when the local
// base lags origin (child rebased onto origin's tip) the origin merge-base is the
// closer one; when the local base is AHEAD of origin (child rebased onto the local
// tip) the local merge-base is the closer one. Blindly preferring origin would
// re-inflate the diff by the local base branch's un-pushed commits. A SHA base
// skips the origin/<sha> probe and yields merge-base(<sha>, HEAD) == <sha> for an
// ancestor SHA, identical to the prior `git diff <forkPointSha>` behaviour.
//
// It returns "" when no candidate ref resolves (an unresolvable/renamed base
// branch, or an empty base): callers treat that as "no committed diff", and the
// app layer never passes an unverified branch name — it falls back to the
// recorded fork point first (see workspace.summaryBase / realtime.watcherDiffBase).
func (e *engine) resolveDiffBase(
	ctx context.Context,
	repoPath string,
	base string,
) string {
	if base == "" {
		return ""
	}
	best := ""
	bestDist := math.MaxInt
	for _, ref := range baseRefCandidates(base) {
		r := e.exec(ctx, repoPath, "merge-base", ref, "HEAD")
		if r.ExitCode != 0 {
			continue
		}
		mergeBase := strings.TrimSpace(r.Stdout)
		if mergeBase == "" {
			continue
		}
		if dist := e.commitDistanceToHead(ctx, repoPath, mergeBase); dist < bestDist {
			best, bestDist = mergeBase, dist
		}
	}
	return best
}

// baseRefCandidates lists the refs resolveDiffBase probes for a base: a branch
// name is checked against both its remote-tracking and local ref (origin-sync
// keeps the former fresh), while a full 40-hex commit SHA is used verbatim — its
// origin/<sha> form would never resolve, so probing it only wastes a subprocess.
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

// commitDistanceToHead returns the number of commits between rev and HEAD
// (`git rev-list --count rev..HEAD`), i.e. how far below HEAD the ancestor rev
// sits. resolveDiffBase uses it to pick the closest of several candidate
// merge-bases. An unreadable count sorts last (math.MaxInt).
func (e *engine) commitDistanceToHead(
	ctx context.Context,
	repoPath string,
	rev string,
) int {
	r := e.exec(ctx, repoPath, "rev-list", "--count", rev+"..HEAD")
	if r.ExitCode != 0 {
		return math.MaxInt
	}
	n, err := strconv.Atoi(strings.TrimSpace(r.Stdout))
	if err != nil {
		return math.MaxInt
	}
	return n
}

func (e *engine) numstatFromBase(
	ctx context.Context,
	repoPath string,
	base string,
) (int, int, error) {
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
