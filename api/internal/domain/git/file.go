package git

// GitFile is a single file entry in a GitStatus (04 §3).
type GitFile struct {
	Path   string        `json:"path"`
	Status GitFileStatus `json:"status"`
	Staged bool          `json:"staged"`
}

// GitFileStatus is the per-file status in a workspace's git state (04 §3).
type GitFileStatus string

const (
	GitFileStatusModified   GitFileStatus = "modified"
	GitFileStatusAdded      GitFileStatus = "added"
	GitFileStatusDeleted    GitFileStatus = "deleted"
	GitFileStatusUntracked  GitFileStatus = "untracked"
	GitFileStatusRenamed    GitFileStatus = "renamed"
	GitFileStatusConflicted GitFileStatus = "conflicted"
)
