package domain

// FileChangeEvent is pushed on the Files topic by the file watcher (05 §5).
type FileChangeEvent struct {
	Type    FileChangeType `json:"type"`
	Path    string         `json:"path"`
	NewPath string         `json:"newPath,omitempty"`
	WsID    string         `json:"wsId"`
}
