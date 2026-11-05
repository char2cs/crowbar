package domain

// FileNodeType distinguishes files from directories in the tree.
type FileNodeType string

const (
	FileNodeTypeFile      FileNodeType = "file"
	FileNodeTypeDirectory FileNodeType = "directory"
)

// FileNodeGitStatus is the git decoration on a file tree node.
// Conflicted files from git are collapsed to Modified for the tree view.
type FileNodeGitStatus string

const (
	FileNodeGitStatusModified  FileNodeGitStatus = "modified"
	FileNodeGitStatusAdded     FileNodeGitStatus = "added"
	FileNodeGitStatusDeleted   FileNodeGitStatus = "deleted"
	FileNodeGitStatusUntracked FileNodeGitStatus = "untracked"
	FileNodeGitStatusRenamed   FileNodeGitStatus = "renamed"
)

// FileNode is a single entry in the lazy file tree.
type FileNode struct {
	Name      string             `json:"name"`
	Path      string             `json:"path"`
	Type      FileNodeType       `json:"type"`
	Children  []FileNode         `json:"children,omitempty"`
	GitStatus *FileNodeGitStatus `json:"gitStatus,omitempty"`
}
