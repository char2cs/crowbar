package workspace

// MergeEligibility is the computed, non-persisted answer to "can this workspace
// be merged into its local parent, and what is that parent's branch" (spec §10).
// It is resolved per read from the sibling set by MergeEligibilityFor; it is never
// stored on the workspace aggregate.
type MergeEligibility struct {
	CanMergeLocally bool
	ParentBranch    string
}
