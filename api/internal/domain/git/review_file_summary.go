package git

// ReviewFileSummary is one entry in the files-only branch-review summary: the
// FULL branch picture for a single path — committed-vs-fork-point changes AND
// working-tree changes — carrying +Additions/-Deletions counts but NO
// line-level hunk content. The whole summary is therefore O(file count) in size
// regardless of how large the underlying diffs are, so the sidebar can show the
// complete changed-files list (with counts) without ever paying for the
// line-level branch diff that the Branch Review pane fetches (Task 27).
//
// Additions/Deletions are -1 for binary files, mirroring git numstat's "-"
// convention (a real 0/0 text change stays distinguishable from a binary one).
type ReviewFileSummary struct {
	Path        string        `json:"path"`
	OldPath     string        `json:"old_path,omitempty"`
	Status      GitFileStatus `json:"status"`
	Additions   int           `json:"additions"`
	Deletions   int           `json:"deletions"`
	Uncommitted bool          `json:"uncommitted"`
	Staged      bool          `json:"staged"`
}
