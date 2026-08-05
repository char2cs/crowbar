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
// repositories (00 §5.1).
type ProjectDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	// Status carries the tombstone marker on the Project broadcast channel: ""
	// for a live project, "deleted" for a removal frame so the FE entity cache
	// drops it (00 §6, one-type-per-channel). Read-path DTOs leave it empty.
	Status       string    `json:"status,omitempty"`
	LastActivity time.Time `json:"lastActivity"`
	// Order is the project's dense index in the sidebar.
	Order int `json:"order"`
	// AvatarURL is the icon proxy "/v0/projects/<id>/icon", set only when the
	// project has an on-disk icon (AvatarHasIcon). Empty otherwise — a project
	// without one falls back to the sidebar's Library glyph rather than to a
	// generated letter tile, which is why there is no label/colour pair here.
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
		// The ?v=<AvatarVersion> query param cache-busts the otherwise-stable
		// icon URL: uploads replace the bytes in place, and without a changing
		// URL the webview's image cache keeps serving the old icon. Same rule,
		// same reason, as RepoDTOFrom.
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

// ProjectDTOList converts a slice of domain Projects into wire DTOs in sidebar
// order, returning a non-nil empty slice when the input is empty so the envelope
// carries [].
//
// The sort lives HERE, in the converter both the REST list handler and the WS
// snapshot go through, because those are the two answers to the same question
// and a client that got different orders from them would watch its sidebar
// reshuffle on every reconnect.
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

// compareProjectDTOs orders projects by their dense index, then by id. The id
// tiebreak keeps a list whose rows all still carry the migration default of 0
// from reshuffling between two identical requests; there is no created-at on a
// project, and lastActivity moves on its own.
func compareProjectDTOs(
	a ProjectDTO,
	b ProjectDTO,
) int {
	if a.Order != b.Order {
		return a.Order - b.Order
	}
	return strings.Compare(a.ID, b.ID)
}
