package lsp

type Location struct {
	FilePath string `json:"filePath"`
	Range    Range  `json:"range"`
}
