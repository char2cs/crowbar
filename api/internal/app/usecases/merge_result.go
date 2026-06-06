package usecases

// MergeResult reports the outcome of a local merge-into-parent (07 §3.1).
// ConflictsPending is true when a git step reported conflicts and a
// pendingMerge marker was set for later resume/abort; in that case ParentTipSha
// is empty. On a clean merge ParentTipSha holds the parent's post-merge HEAD,
// to which the kept child's fork point has been advanced.
type MergeResult struct {
	ConflictsPending bool
	ParentTipSha     string
}
