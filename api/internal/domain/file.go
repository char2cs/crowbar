package domain

// FileContent is the response from a file-content read.
// Encoding is "base64" for binary files; empty for UTF-8 text.
type FileContent struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding,omitempty"`
}

// FileChangeType classifies a filesystem event.
type FileChangeType string

const (
	FileChangeCreated  FileChangeType = "created"
	FileChangeModified FileChangeType = "modified"
	FileChangeDeleted  FileChangeType = "deleted"
	FileChangeRenamed  FileChangeType = "renamed"
)

// FileChangeEvent is pushed on the Files topic by the file watcher,
// and on the documents stream for a held host document.
type FileChangeEvent struct {
	Type      FileChangeType    `json:"type"`
	Authority DocumentAuthority `json:"authority"`
	// Path is workspace-relative, or the held absolute path of a host document.
	Path    string `json:"path"`
	NewPath string `json:"newPath,omitempty"`
	// WsID is "" for a host document.
	WsID string `json:"wsId"`
}
