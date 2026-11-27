package git

type GitFile struct {
	Path   string        `json:"path"`
	Status GitFileStatus `json:"status"`
	Staged bool          `json:"staged"`
}

type GitFileStatus string

const (
	GitFileStatusModified   GitFileStatus = "modified"
	GitFileStatusAdded      GitFileStatus = "added"
	GitFileStatusDeleted    GitFileStatus = "deleted"
	GitFileStatusUntracked  GitFileStatus = "untracked"
	GitFileStatusRenamed    GitFileStatus = "renamed"
	GitFileStatusConflicted GitFileStatus = "conflicted"
)
