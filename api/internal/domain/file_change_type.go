package domain

// FileChangeType classifies a filesystem event (05 §5).
type FileChangeType string

const (
	FileChangeCreated FileChangeType = "created"
	FileChangeModified FileChangeType = "modified"
	FileChangeDeleted FileChangeType = "deleted"
	FileChangeRenamed FileChangeType = "renamed"
)
