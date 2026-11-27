package domain

type FileNodeType string

const (
	FileNodeTypeFile      FileNodeType = "file"
	FileNodeTypeDirectory FileNodeType = "directory"
)

type FileNodeGitStatus string

const (
	FileNodeGitStatusModified  FileNodeGitStatus = "modified"
	FileNodeGitStatusAdded     FileNodeGitStatus = "added"
	FileNodeGitStatusDeleted   FileNodeGitStatus = "deleted"
	FileNodeGitStatusUntracked FileNodeGitStatus = "untracked"
	FileNodeGitStatusRenamed   FileNodeGitStatus = "renamed"
)

type FileNode struct {
	Name      string             `json:"name"`
	Path      string             `json:"path"`
	Type      FileNodeType       `json:"type"`
	Children  []FileNode         `json:"children,omitempty"`
	GitStatus *FileNodeGitStatus `json:"gitStatus,omitempty"`
}
