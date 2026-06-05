package domain

// FileNodeType distinguishes files from directories in the tree (05 §2).
type FileNodeType string

const (
	FileNodeTypeFile      FileNodeType = "file"
	FileNodeTypeDirectory FileNodeType = "directory"
)
