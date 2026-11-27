package lsp

type Diagnostic struct {
	FilePath string `json:"filePath"`
	Range    Range  `json:"range"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Source   string `json:"source,omitempty"`
	Code     string `json:"code,omitempty"`
}

type DiagnosticsEvent struct {
	WsID        string       `json:"wsId"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}
