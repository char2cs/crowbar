package dto

import (
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// BlameEntryDTO is the wire shape of a single blame annotation (04 §3): the
// line number, the last-changing commit hash, the author identity, the commit
// timestamp, and the commit subject.
type BlameEntryDTO struct {
	LineNumber    int    `json:"lineNumber"`
	CommitHash    string `json:"commitHash"`
	Author        string `json:"author"`
	Email         string `json:"email"`
	Date          string `json:"date"`
	CommitMessage string `json:"commitMessage"`
}

// BlameEntryDTOFrom converts a domain BlameEntry into its wire DTO, rendering
// the commit timestamp in RFC 3339.
func BlameEntryDTOFrom(
	entry gitdomain.BlameEntry,
) BlameEntryDTO {
	return BlameEntryDTO{
		LineNumber:    entry.LineNumber,
		CommitHash:    entry.CommitHash,
		Author:        entry.Author,
		Email:         entry.Email,
		Date:          entry.Date.Format("2006-01-02T15:04:05Z07:00"),
		CommitMessage: entry.CommitMessage,
	}
}

// BlameEntryDTOList converts a slice of domain BlameEntries into wire DTOs. It
// returns a non-nil empty slice for empty input so the envelope's data field
// serialises as [] rather than null.
func BlameEntryDTOList(
	entries []gitdomain.BlameEntry,
) []BlameEntryDTO {
	out := make([]BlameEntryDTO, 0, len(entries))
	for _, entry := range entries {
		out = append(out, BlameEntryDTOFrom(entry))
	}
	return out
}
