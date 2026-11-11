// Package lsp contains the Crowbar LSP domain DTOs used across the engine and
// API layers.
package lsp

// Position is a 0-based line/character coordinate.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a start/end Position pair.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}
