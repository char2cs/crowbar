// Package lsp contains the Crowbar LSP domain DTOs used across the engine and
// API layers (10 §6).
package lsp

// Position is a 0-based line/character coordinate (10 §6).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a start/end Position pair.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}
