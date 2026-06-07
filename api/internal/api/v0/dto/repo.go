package dto

import (
	"github.com/char2cs/crowbar/api/internal/domain"
)

// RepoDTO is the wire shape of a Repository: a git repo imported under a
// Project (00 §5.2). The workspace tree is intentionally absent: no usecase
// currently composes it, so repo detail returns the repo fields only.
type RepoDTO struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"defaultBranch"`
	AvatarLabel   string `json:"avatarLabel"`
	AvatarColor   string `json:"avatarColor"`
}

// RepoDTOFrom converts a domain Repository into its wire DTO.
func RepoDTOFrom(
	r domain.Repository,
) RepoDTO {
	return RepoDTO{
		ID:            r.ID,
		ProjectID:     r.ProjectID,
		Name:          r.Name,
		Path:          r.Path,
		DefaultBranch: r.DefaultBranch,
		AvatarLabel:   r.AvatarLabel,
		AvatarColor:   r.AvatarColor,
	}
}

// RepoDTOList converts a slice of domain Repositories into wire DTOs, returning
// a non-nil empty slice when the input is empty so the envelope carries [].
func RepoDTOList(
	repos []domain.Repository,
) []RepoDTO {
	dtos := make([]RepoDTO, 0, len(repos))
	for _, r := range repos {
		dtos = append(dtos, RepoDTOFrom(r))
	}
	return dtos
}
