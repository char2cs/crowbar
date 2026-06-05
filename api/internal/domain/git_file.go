package domain

// GitFile is a single file entry in a GitStatus (04 §3).
type GitFile struct {
	Path   string        `json:"path"`
	Status GitFileStatus `json:"status"`
	Staged bool          `json:"staged"`
}
