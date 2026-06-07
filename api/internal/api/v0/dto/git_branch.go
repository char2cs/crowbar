package dto

import (
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// BranchDTO is the wire shape of a single git branch (04 §3): its name, whether
// it is the checked-out branch, whether it is a remote-tracking ref, its
// optional ahead/behind counts against the upstream, and the timestamp of its
// last commit in RFC 3339.
type BranchDTO struct {
	Name           string  `json:"name"`
	IsCurrent      bool    `json:"isCurrent"`
	IsRemote       bool    `json:"isRemote"`
	Ahead          *int    `json:"ahead,omitempty"`
	Behind         *int    `json:"behind,omitempty"`
	LastCommitDate *string `json:"lastCommitDate,omitempty"`
}

// BranchDTOFrom converts a domain Branch into its wire DTO, rendering the
// last-commit timestamp in RFC 3339 when present.
func BranchDTOFrom(
	branch gitdomain.Branch,
) BranchDTO {
	out := BranchDTO{
		Name:      branch.Name,
		IsCurrent: branch.IsCurrent,
		IsRemote:  branch.IsRemote,
		Ahead:     branch.Ahead,
		Behind:    branch.Behind,
	}
	if branch.LastCommitDate != nil {
		date := branch.LastCommitDate.Format("2006-01-02T15:04:05Z07:00")
		out.LastCommitDate = &date
	}
	return out
}

// BranchDTOList converts a slice of domain Branches into wire DTOs. It returns a
// non-nil empty slice for empty input so the envelope's data field serialises as
// [] rather than null.
func BranchDTOList(
	branches []gitdomain.Branch,
) []BranchDTO {
	out := make([]BranchDTO, 0, len(branches))
	for _, branch := range branches {
		out = append(out, BranchDTOFrom(branch))
	}
	return out
}
