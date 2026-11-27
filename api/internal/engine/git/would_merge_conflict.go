package git

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type MergePair struct {
	Ours   string
	Theirs string
}

type MergeVerdict struct {
	Conflict bool
	Err      error
}

const mergeVerdictCap = 4096

const mergeDryRunConcurrency = 4

type commitPair struct{ ours, theirs string }

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

func (e *engine) WouldMergeConflicts(
	ctx context.Context,
	repoPath string,
	pairs []MergePair,
) []MergeVerdict {
	out := make([]MergeVerdict, len(pairs))
	defer e.lockRepoRead(ctx, repoPath)()
	tips := e.branchTips(ctx, repoPath)

	var wg sync.WaitGroup
	slots := make(chan struct{}, mergeDryRunConcurrency)
	for i, p := range pairs {
		ours, theirs, known := resolveTips(tips, p)
		key := commitPair{ours, theirs}
		if conflict, hit := e.verdicts.get(key); known && hit {
			out[i] = MergeVerdict{Conflict: conflict}
			continue
		}
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			conflict, err := e.mergeTreeConflicts(ctx, repoPath, ours, theirs)
			out[i] = MergeVerdict{Conflict: conflict, Err: err}
			if err == nil && known {
				e.verdicts.put(key, conflict)
			}
		})
	}
	wg.Wait()
	return out
}

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
