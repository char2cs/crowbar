package git

import (
	"context"
	"fmt"
	"strings"
)

// WouldMergeConflict reports whether a three-way merge of theirs into ours would
// produce conflicts, computed non-destructively with `git merge-tree
// --write-tree` — no worktree, index, or refs are touched, only loose objects
// are written.
//
// Exit-code handling matters: a CLEAN merge exits 0; a genuine CONFLICT exits 1
// AND still writes the merged tree OID to stdout. But a FAILURE to run — a
// missing worktree dir, an unresolvable ref — also surfaces as a non-zero exit
// (1 or 128) with EMPTY stdout. So a bare "exit==1 means conflict" check would
// fail CLOSED (wrongly block a clean merge) on those error paths. We only treat
// exit 1 as a conflict when git actually produced a tree; everything else
// non-zero is returned as an error so the caller can fail OPEN.
func (e *engine) WouldMergeConflict(
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
