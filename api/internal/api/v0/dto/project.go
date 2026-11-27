package dto

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ProjectDTO struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Status       string    `json:"status,omitempty"`
	LastActivity time.Time `json:"lastActivity"`
	Order        int       `json:"order"`
	AvatarURL    string    `json:"avatarUrl,omitempty"`
	AvatarEmoji  string    `json:"avatarEmoji,omitempty"`
}

func ProjectDTOFrom(
	p domain.Project,
) ProjectDTO {
	avatarURL := ""
	if p.AvatarHasIcon {
		avatarURL = "/v0/projects/" + p.ID + "/icon?v=" + strconv.FormatInt(p.AvatarVersion, 10)
	}
	return ProjectDTO{
		ID:           p.ID,
		Name:         p.Name,
		Path:         p.Path,
		LastActivity: p.LastActivity,
		Order:        p.Order,
		AvatarURL:    avatarURL,
		AvatarEmoji:  p.AvatarEmoji,
	}
}

func ProjectDTOList(
	projects []domain.Project,
) []ProjectDTO {
	dtos := make([]ProjectDTO, 0, len(projects))
	for _, p := range projects {
		dtos = append(dtos, ProjectDTOFrom(p))
	}
	slices.SortFunc(dtos, compareProjectDTOs)
	return dtos
}

func compareProjectDTOs(
	a ProjectDTO,
	b ProjectDTO,
) int {
	if a.Order != b.Order {
		return a.Order - b.Order
	}
	return strings.Compare(a.ID, b.ID)
}
