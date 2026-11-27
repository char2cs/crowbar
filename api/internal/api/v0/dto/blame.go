package dto

import (
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type BlameEntryDTO struct {
	LineNumber    int    `json:"lineNumber"`
	CommitHash    string `json:"commitHash"`
	Author        string `json:"author"`
	Email         string `json:"email"`
	Date          string `json:"date"`
	CommitMessage string `json:"commitMessage"`
}

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

func BlameEntryDTOList(
	entries []gitdomain.BlameEntry,
) []BlameEntryDTO {
	out := make([]BlameEntryDTO, 0, len(entries))
	for _, entry := range entries {
		out = append(out, BlameEntryDTOFrom(entry))
	}
	return out
}
