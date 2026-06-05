package domain

// FileNodeGitStatus is the git decoration on a file tree node (05 §2).
// Conflicted files from git are collapsed to Modified for the tree view.
type FileNodeGitStatus string

const (
	FileNodeGitStatusModified  FileNodeGitStatus = "modified"
	FileNodeGitStatusAdded     FileNodeGitStatus = "added"
	FileNodeGitStatusDeleted   FileNodeGitStatus = "deleted"
	FileNodeGitStatusUntracked FileNodeGitStatus = "untracked"
	FileNodeGitStatusRenamed   FileNodeGitStatus = "renamed"
)
