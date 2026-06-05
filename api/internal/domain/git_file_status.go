package domain

// GitFileStatus is the per-file status in a workspace's git state (04 §3).
type GitFileStatus string

const (
	GitFileStatusModified  GitFileStatus = "modified"
	GitFileStatusAdded     GitFileStatus = "added"
	GitFileStatusDeleted   GitFileStatus = "deleted"
	GitFileStatusUntracked GitFileStatus = "untracked"
	GitFileStatusRenamed   GitFileStatus = "renamed"
	GitFileStatusConflicted GitFileStatus = "conflicted"
)
