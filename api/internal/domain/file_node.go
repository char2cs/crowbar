package domain

// FileNode is a single entry in the lazy file tree (05 §2).
type FileNode struct {
	Name      string             `json:"name"`
	Path      string             `json:"path"`
	Type      FileNodeType       `json:"type"`
	Children  []FileNode         `json:"children,omitempty"`
	GitStatus *FileNodeGitStatus `json:"gitStatus,omitempty"`
}
