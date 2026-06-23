package worktree

// MergeResult reports the outcome of a local merge-into-parent (07 §3.1).
// ConflictsPending is true when a git step reported conflicts: the in-progress
// merge/rebase is ABORTED (try-then-warn, never a stuck worktree) and the
// conflict is surfaced as the child's pr-conflicts state — there is NO
// resumable marker. The user resolves it via "Rebase onto parent" (a resolvable
// rebase kept in the child's own worktree) and re-runs the merge once clean. In
// that case ParentTipSha is empty. On a clean merge ParentTipSha holds the
// parent's post-merge HEAD, to which the kept child's fork point has been
// advanced.
type MergeResult struct {
	ConflictsPending bool   `json:"conflictsPending"`
	ParentTipSha     string `json:"parentTipSha"`
}
