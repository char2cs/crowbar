package lsp

// Location is a file path plus a Range within that file.
type Location struct {
	FilePath string `json:"filePath"`
	Range    Range  `json:"range"`
}
