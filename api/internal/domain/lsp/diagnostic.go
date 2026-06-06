package lsp

// Diagnostic is a single editor diagnostic pushed over the LSP WS topic (10 §2).
type Diagnostic struct {
	FilePath string `json:"filePath"`
	Range    Range  `json:"range"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Source   string `json:"source,omitempty"`
	Code     string `json:"code,omitempty"`
}

// DiagnosticsEvent is the wsId-scoped batch payload for the broadcaster (03 §3).
type DiagnosticsEvent struct {
	WsID        string       `json:"wsId"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}
