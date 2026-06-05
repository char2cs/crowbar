package domain

// PendingMerge records a conflicted merge-into-parent awaiting resolution (07 §3.1).
type PendingMerge struct {
	Strategy       MergeStrategy `json:"strategy"`
	TargetParentID string        `json:"targetParentId"`
}
