package domain

// DiffLine is one line in a FileDiff's flat rendering. Changed lines carry
// hunkId linking them to their Hunk (04 §4).
type DiffLine struct {
	LineType      DiffLineType `json:"line_type"`
	Content       string       `json:"content"`
	OldLineNumber *int         `json:"old_line_number,omitempty"`
	NewLineNumber *int         `json:"new_line_number,omitempty"`
	HunkID        string       `json:"hunkId,omitempty"`
}
