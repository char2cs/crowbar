package dto

import (
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// StashDTO is the wire shape of a single stash entry (04 §3): its stash ref id,
// its message, the timestamp it was created in RFC 3339, and the number of files
// it touched.
type StashDTO struct {
	ID           string `json:"id"`
	Message      string `json:"message"`
	Date         string `json:"date"`
	FilesChanged int    `json:"filesChanged"`
}

// StashDTOFrom converts a domain Stash into its wire DTO, rendering the creation
// timestamp in RFC 3339.
func StashDTOFrom(
	stash gitdomain.Stash,
) StashDTO {
	return StashDTO{
		ID:           stash.ID,
		Message:      stash.Message,
		Date:         stash.Date.Format("2006-01-02T15:04:05Z07:00"),
		FilesChanged: stash.FilesChanged,
	}
}

// StashDTOList converts a slice of domain Stashes into wire DTOs. It returns a
// non-nil empty slice for empty input so the envelope's data field serialises as
// [] rather than null.
func StashDTOList(
	stashes []gitdomain.Stash,
) []StashDTO {
	out := make([]StashDTO, 0, len(stashes))
	for _, stash := range stashes {
		out = append(out, StashDTOFrom(stash))
	}
	return out
}
