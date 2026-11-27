package domain

type FileContent struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding,omitempty"`
}

type FileChangeType string

const (
	FileChangeCreated  FileChangeType = "created"
	FileChangeModified FileChangeType = "modified"
	FileChangeDeleted  FileChangeType = "deleted"
	FileChangeRenamed  FileChangeType = "renamed"
)

type FileChangeEvent struct {
	Type      FileChangeType    `json:"type"`
	Authority DocumentAuthority `json:"authority"`
	Path      string            `json:"path"`
	NewPath   string            `json:"newPath,omitempty"`
	WsID      string            `json:"wsId"`
}
