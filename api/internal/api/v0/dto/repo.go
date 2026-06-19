package dto

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type RepoDTO struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"defaultBranch"`
	AvatarLabel   string `json:"avatarLabel"`
	AvatarColor   string `json:"avatarColor"`
	AvatarURL     string `json:"avatarUrl,omitempty"`
}

func RepoDTOFrom(r domain.Repository) RepoDTO {
	avatarURL := r.AvatarURL
	switch {
	case avatarURL == "":
		// no change
	case strings.HasPrefix(avatarURL, "emoji:"):
		// pass through; frontend renders emoji directly
	default:
		// local file path or HTTPS URL — always proxy through the API so
		// WKWebView (crowbar:// scheme) can load it without cross-origin issues
		avatarURL = "/v0/repos/" + r.ID + "/icon"
	}
	return RepoDTO{
		ID:            r.ID,
		ProjectID:     r.ProjectID,
		Name:          r.Name,
		Path:          r.Path,
		DefaultBranch: r.DefaultBranch,
		AvatarLabel:   r.AvatarLabel,
		AvatarColor:   r.AvatarColor,
		AvatarURL:     avatarURL,
	}
}

func RepoDTOList(repos []domain.Repository) []RepoDTO {
	dtos := make([]RepoDTO, 0, len(repos))
	for _, r := range repos {
		dtos = append(dtos, RepoDTOFrom(r))
	}
	return dtos
}
