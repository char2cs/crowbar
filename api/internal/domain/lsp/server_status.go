package lsp

type ServerState string

const (
	ServerUnsupported  ServerState = "unsupported"
	ServerNotInstalled ServerState = "notInstalled"
	ServerStopped      ServerState = "stopped"
	ServerRunning      ServerState = "running"
)

type ServerStatus struct {
	LanguageID string      `json:"languageId,omitempty"`
	Command    string      `json:"command,omitempty"`
	State      ServerState `json:"state"`
}

type FormattingOptions struct {
	TabSize      int  `json:"tabSize"`
	InsertSpaces bool `json:"insertSpaces"`
}
