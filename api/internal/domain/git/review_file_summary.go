package git

// ReviewFileSummary is one path's full branch picture (committed and working-tree)
// with +/- counts but no hunks, so the files list stays O(file count). Additions and
// Deletions are -1 for binary files, like numstat's "-".
type ReviewFileSummary struct {
	Path        string        `json:"path"`
	OldPath     string        `json:"old_path,omitempty"`
	Status      GitFileStatus `json:"status"`
	Additions   int           `json:"additions"`
	Deletions   int           `json:"deletions"`
	Uncommitted bool          `json:"uncommitted"`
	Staged      bool          `json:"staged"`
}
