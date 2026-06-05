package domain

// Hunk is a contiguous changed block in a FileDiff. StartLine and EndLine are
// inclusive indices into FileDiff.Lines (04 §4).
type Hunk struct {
	HunkID    string `json:"hunkId"`
	Header    string `json:"header"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}
