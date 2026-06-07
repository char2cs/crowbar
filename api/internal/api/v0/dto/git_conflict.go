package dto

import (
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// ConflictHunkDTO is the wire shape of a single three-way conflict block (04
// §6): its stable id, the line range it spans, the ours/theirs/base sides, the
// chosen resolution, and any custom resolved content.
type ConflictHunkDTO struct {
	ID              string `json:"id"`
	StartLine       int    `json:"startLine"`
	EndLine         int    `json:"endLine"`
	Ours            string `json:"ours"`
	Theirs          string `json:"theirs"`
	Base            string `json:"base,omitempty"`
	Resolution      string `json:"resolution"`
	ResolvedContent string `json:"resolvedContent,omitempty"`
}

// ConflictFileDTO is the wire shape of a single conflicting file (04 §6): its
// workspace-relative path and the conflict hunks within it.
type ConflictFileDTO struct {
	Path  string            `json:"path"`
	Hunks []ConflictHunkDTO `json:"hunks"`
}

// ConflictHunkDTOFrom converts a domain ConflictHunk into its wire DTO.
func ConflictHunkDTOFrom(
	hunk gitdomain.ConflictHunk,
) ConflictHunkDTO {
	return ConflictHunkDTO{
		ID:              hunk.ID,
		StartLine:       hunk.StartLine,
		EndLine:         hunk.EndLine,
		Ours:            hunk.Ours,
		Theirs:          hunk.Theirs,
		Base:            hunk.Base,
		Resolution:      string(hunk.Resolution),
		ResolvedContent: hunk.ResolvedContent,
	}
}

// ConflictHunkDTOList converts a slice of domain ConflictHunks into wire DTOs.
// It returns a non-nil empty slice for empty input so the field serialises as
// [] rather than null.
func ConflictHunkDTOList(
	hunks []gitdomain.ConflictHunk,
) []ConflictHunkDTO {
	out := make([]ConflictHunkDTO, 0, len(hunks))
	for _, hunk := range hunks {
		out = append(out, ConflictHunkDTOFrom(hunk))
	}
	return out
}

// ConflictFileDTOFrom builds a ConflictFileDTO from a path and its conflict
// hunks, rendering an empty but non-nil hunk slice when the file has none.
func ConflictFileDTOFrom(
	path string,
	hunks []gitdomain.ConflictHunk,
) ConflictFileDTO {
	return ConflictFileDTO{
		Path:  path,
		Hunks: ConflictHunkDTOList(hunks),
	}
}
