package lsp

// ServerState is the lifecycle state of the language server that serves a
// file's language in one workspace.
type ServerState string

const (
	// ServerUnsupported means no language server is configured for the file's
	// extension.
	ServerUnsupported ServerState = "unsupported"
	// ServerNotInstalled means a server is configured but its binary is not on
	// PATH.
	ServerNotInstalled ServerState = "notInstalled"
	// ServerStopped means the server is installed but not running; it starts on
	// the next document open or feature request.
	ServerStopped ServerState = "stopped"
	// ServerRunning means a server process is live for the workspace.
	ServerRunning ServerState = "running"
)

// ServerStatus reports which language server serves a file and whether it is
// running.
type ServerStatus struct {
	LanguageID string      `json:"languageId,omitempty"`
	Command    string      `json:"command,omitempty"`
	State      ServerState `json:"state"`
}

// FormattingOptions mirrors LSP's FormattingOptions (the subset editors send).
type FormattingOptions struct {
	TabSize      int  `json:"tabSize"`
	InsertSpaces bool `json:"insertSpaces"`
}
