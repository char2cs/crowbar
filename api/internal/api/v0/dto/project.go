// Package dto holds the v0 REST data-transfer types and their converters from
// the internal domain entities. Handlers serialize DTOs — never the domain
// types directly — so the wire shape stays decoupled from persistence.
package dto

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ProjectDTO is the wire shape of a Project: the org-level node grouping
// repositories.
type ProjectDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	// Status carries the tombstone marker on the Project broadcast channel: ""
	// for a live project, "deleted" for a removal frame so the FE entity cache
	// drops it (one type per channel). Read-path DTOs leave it empty.
	Status       string    `json:"status,omitempty"`
	LastActivity time.Time `json:"lastActivity"`
	// Order is the project's dense index in the sidebar.
	Order int `json:"order"`
	// AvatarURL is the icon proxy "/v0/projects/<id>/icon", set only for an on-disk
	// icon; otherwise the sidebar shows its Library glyph, so there is no label/colour pair.
	AvatarURL string `json:"avatarUrl,omitempty"`
	// AvatarEmoji passes the emoji icon through to the client, which renders it
	// directly. Empty when the project uses an on-disk image or the default.
	AvatarEmoji string `json:"avatarEmoji,omitempty"`
}

// ProjectDTOFrom converts a domain Project into its wire DTO.
func ProjectDTOFrom(
	p domain.Project,
) ProjectDTO {
	avatarURL := ""
	if p.AvatarHasIcon {
		// ?v=<AvatarVersion> cache-busts the stable icon URL: uploads replace the bytes
		// in place, and the webview's image cache would keep serving the old icon.
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

// ProjectDTOList converts projects into a non-nil DTO slice in sidebar order. The sort
// lives here because REST and the WS snapshot both use it, and differing orders would
// reshuffle the sidebar on every reconnect.
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

// compareProjectDTOs orders projects by dense index, then id, so rows all still at
// the migration default of 0 stay stable (projects have no created-at to break ties).
func compareProjectDTOs(
	a ProjectDTO,
	b ProjectDTO,
) int {
	if a.Order != b.Order {
		return a.Order - b.Order
	}
	return strings.Compare(a.ID, b.ID)
}
