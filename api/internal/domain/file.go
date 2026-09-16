package domain

// FileContent is the response from a file-content read (05 §3).
// Encoding is "base64" for binary files; empty for UTF-8 text.
type FileContent struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding,omitempty"`
}

// FileChangeType classifies a filesystem event (05 §5).
type FileChangeType string

const (
	FileChangeCreated  FileChangeType = "created"
	FileChangeModified FileChangeType = "modified"
	FileChangeDeleted  FileChangeType = "deleted"
	FileChangeRenamed  FileChangeType = "renamed"
)

// FileChangeEvent is pushed on the Files topic by the file watcher (05 §5).
//
// ChatIDs answers the scoping question the chat-scoped route
// /v0/chats/:chatId/files/ws asks: every chat whose worktree resolves to WsID,
// as of this push (spec §7.4). Files is the shared bucket — one worktree, one
// tree — so a change made through ONE chat is news for every sibling chat
// holding that worktree, and carrying the set on the event is what lets one
// Push reach all of them in a single fan-out pass.
//
// It is routing, not payload, and is the one field here kept off the wire: this
// topic serializes the whole event, so a consumer would otherwise be handed a
// workspace's chat roster with every file save.
type FileChangeEvent struct {
	Type    FileChangeType `json:"type"`
	Path    string         `json:"path"`
	NewPath string         `json:"newPath,omitempty"`
	WsID    string         `json:"wsId"`
	ChatIDs []string       `json:"-"`
}
