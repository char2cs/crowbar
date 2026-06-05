package domain

// FileContent is the response from a file-content read (05 §3).
// Encoding is "base64" for binary files; empty for UTF-8 text.
type FileContent struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding,omitempty"`
}
