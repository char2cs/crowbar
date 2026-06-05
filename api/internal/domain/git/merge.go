package git

// MergeStrategy is the branch-review merge selector (09 §4).
type MergeStrategy string

const (
	MergeStrategyMerge  MergeStrategy = "merge"
	MergeStrategySquash MergeStrategy = "squash"
	MergeStrategyRebase MergeStrategy = "rebase"
)

// PendingMerge records a conflicted merge-into-parent awaiting resolution (07 §3.1).
type PendingMerge struct {
	Strategy       MergeStrategy `json:"strategy"`
	TargetParentID string        `json:"targetParentId"`
}
