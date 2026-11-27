package git

type ReviewFileSummary struct {
	Path        string        `json:"path"`
	OldPath     string        `json:"old_path,omitempty"`
	Status      GitFileStatus `json:"status"`
	Additions   int           `json:"additions"`
	Deletions   int           `json:"deletions"`
	Uncommitted bool          `json:"uncommitted"`
	Staged      bool          `json:"staged"`
}
