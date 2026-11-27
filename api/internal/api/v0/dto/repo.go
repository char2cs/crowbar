package dto

import (
	"slices"
	"strconv"
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
	AvatarEmoji   string `json:"avatarEmoji,omitempty"`
	Status        string `json:"status,omitempty"`
	Order         int    `json:"order"`
	FolderID      string `json:"folderId,omitempty"`
	Deleting      bool   `json:"deleting,omitempty"`
}

type RepoPlacement struct {
	FolderID string
	Order    int
}

func RepoDTOFrom(r domain.Repository, placement RepoPlacement) RepoDTO {
	avatarURL := ""
	if r.AvatarHasIcon {
		avatarURL = "/v0/projects/" + r.ProjectID + "/repos/" + r.ID + "/icon" +
			"?v=" + strconv.FormatInt(r.AvatarVersion, 10)
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
		AvatarEmoji:   r.AvatarEmoji,
		Order:         placement.Order,
		FolderID:      placement.FolderID,
		Deleting:      r.Deleting,
	}
}

func RepoDTOList(repos []domain.Repository, placements map[string]RepoPlacement) []RepoDTO {
	dtos := make([]RepoDTO, 0, len(repos))
	for _, r := range repos {
		dtos = append(dtos, RepoDTOFrom(r, placements[r.ID]))
	}
	slices.SortFunc(dtos, compareRepoDTOs)
	return dtos
}

func compareRepoDTOs(
	a RepoDTO,
	b RepoDTO,
) int {
	if a.Order != b.Order {
		return a.Order - b.Order
	}
	return strings.Compare(a.ID, b.ID)
}
