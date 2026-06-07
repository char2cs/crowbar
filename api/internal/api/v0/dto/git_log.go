package dto

import (
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// CommitDTO is the wire shape of a single git-log entry (04 §3): the full and
// abbreviated hashes, the subject and optional body, the author identity, and
// the commit timestamp rendered in RFC 3339.
type CommitDTO struct {
	Hash        string `json:"hash"`
	ShortHash   string `json:"shortHash"`
	Message     string `json:"message"`
	Description string `json:"description,omitempty"`
	Author      string `json:"author"`
	Email       string `json:"email,omitempty"`
	Date        string `json:"date"`
}

// CommitDTOFrom converts a domain Commit into its wire DTO, rendering the commit
// timestamp in RFC 3339.
func CommitDTOFrom(
	commit gitdomain.Commit,
) CommitDTO {
	return CommitDTO{
		Hash:        commit.Hash,
		ShortHash:   commit.ShortHash,
		Message:     commit.Message,
		Description: commit.Description,
		Author:      commit.Author,
		Email:       commit.Email,
		Date:        commit.Date.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// CommitDTOList converts a slice of domain Commits into wire DTOs. It returns a
// non-nil empty slice for empty input so the envelope's data field serialises as
// [] rather than null.
func CommitDTOList(
	commits []gitdomain.Commit,
) []CommitDTO {
	out := make([]CommitDTO, 0, len(commits))
	for _, commit := range commits {
		out = append(out, CommitDTOFrom(commit))
	}
	return out
}
