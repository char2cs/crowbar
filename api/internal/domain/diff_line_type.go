package domain

// DiffLineType classifies a single line in a unified diff (04 §3).
type DiffLineType string

const (
	DiffLineAdded   DiffLineType = "added"
	DiffLineRemoved DiffLineType = "removed"
	DiffLineContext DiffLineType = "context"
	DiffLineHeader  DiffLineType = "header"
)
