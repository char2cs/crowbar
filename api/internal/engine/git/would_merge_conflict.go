package git

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// MergePair is one dry-run: whether merging Theirs into Ours would conflict,
// both named as branches.
type MergePair struct {
	Ours   string
	Theirs string
}

// MergeVerdict answers one MergePair. A non-nil Err means the check could not
// run (an unresolvable branch, a missing repo); the caller fails OPEN, and
// Conflict is then false.
type MergeVerdict struct {
	Conflict bool
	Err      error
}

// mergeVerdictCap bounds the verdict memory. Verdicts are facts about a pair of
// commits, so nothing ever invalidates one; the cap alone keeps it bounded.
const mergeVerdictCap = 4096

// mergeDryRunConcurrency bounds the merge-trees one batch runs at once.
const mergeDryRunConcurrency = 4

type commitPair struct{ ours, theirs string }

// mergeVerdicts remembers merge-tree results by the two COMMITS merged, never by
// branch name: a branch that moves names different commits and so misses.
type mergeVerdicts struct {
	mu sync.Mutex
	m  map[commitPair]bool
}

func (c *mergeVerdicts) get(k commitPair) (conflict, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	conflict, ok = c.m[k]
	return conflict, ok
}

func (c *mergeVerdicts) put(k commitPair, conflict bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) >= mergeVerdictCap {
		c.m = make(map[commitPair]bool, mergeVerdictCap)
	}
	c.m[k] = conflict
}

// mergeTreeConflicts is the merge-tree dry-run: ours and theirs merged with
// `git merge-tree --write-tree`, touching no worktree, index or ref — only loose
// objects are written. The caller holds the repo's read lock.
//
// Exit-code handling matters: a CLEAN merge exits 0; a genuine CONFLICT exits 1
// AND still writes the merged tree OID to stdout. But a FAILURE to run — a
// missing worktree dir, an unresolvable ref — also surfaces as a non-zero exit
// (1 or 128) with EMPTY stdout. So a bare "exit==1 means conflict" check would
// fail CLOSED (wrongly block a clean merge) on those error paths. We only treat
// exit 1 as a conflict when git actually produced a tree; everything else
// non-zero is an error the caller fails OPEN on.
func (e *engine) mergeTreeConflicts(
	ctx context.Context,
	repoPath string,
	ours string,
	theirs string,
) (bool, error) {
	r := e.exec(ctx, repoPath, "merge-tree", "--write-tree", ours, theirs)
	if r.ExitCode == 0 {
		return false, nil
	}
	if r.ExitCode == 1 && strings.TrimSpace(r.Stdout) != "" {
		return true, nil
	}
	return false, fmt.Errorf(
		"merge-tree %s %s: exit %d: %s", ours, theirs, r.ExitCode, strings.TrimSpace(r.Stderr),
	)
}

// WouldMergeConflicts answers every pair against ONE read of the repo's branch
// tips. A pair whose two commits were already dry-run is answered from memory,
// so a repeated list pays one git process, not one per branch; the rest run
// concurrently, bounded. A branch that is not among the tips is dry-run by name.
func (e *engine) WouldMergeConflicts(
	ctx context.Context,
	repoPath string,
	pairs []MergePair,
) []MergeVerdict {
	out := make([]MergeVerdict, len(pairs))
	defer e.lockRepoRead(ctx, repoPath)()
	tips := e.branchTips(ctx, repoPath)

	var g errgroup.Group
	g.SetLimit(mergeDryRunConcurrency)
	for i, p := range pairs {
		ours, theirs, known := resolveTips(tips, p)
		key := commitPair{ours, theirs}
		if conflict, hit := e.verdicts.get(key); known && hit {
			out[i] = MergeVerdict{Conflict: conflict}
			continue
		}
		g.Go(func() error {
			conflict, err := e.mergeTreeConflicts(ctx, repoPath, ours, theirs)
			out[i] = MergeVerdict{Conflict: conflict, Err: err}
			if err == nil && known {
				e.verdicts.put(key, conflict)
			}
			return nil
		})
	}
	_ = g.Wait() // workers never return an error
	return out
}

// resolveTips names p's two commits, or — when either branch is not among the
// tips — falls back to the branch names, known false so nothing is remembered.
func resolveTips(
	tips map[string]string,
	p MergePair,
) (ours, theirs string, known bool) {
	ours, okOurs := tips[p.Ours]
	theirs, okTheirs := tips[p.Theirs]
	if okOurs && okTheirs {
		return ours, theirs, true
	}
	return p.Ours, p.Theirs, false
}
