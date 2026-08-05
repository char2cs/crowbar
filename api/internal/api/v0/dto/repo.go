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
	// AvatarURL is the hierarchical icon proxy
	// "/v0/projects/<projectId>/repos/<id>/icon", set only when the repo has an
	// on-disk icon (AvatarHasIcon). Empty otherwise.
	AvatarURL string `json:"avatarUrl,omitempty"`
	// AvatarEmoji passes the emoji icon through to the client, which renders it
	// directly. Empty when the repo uses an on-disk image or a generated avatar.
	AvatarEmoji string `json:"avatarEmoji,omitempty"`
	// Status carries the tombstone marker on the Repo broadcast channel: "" for a
	// live repo, "deleted" for a removal frame so the FE entity cache drops it
	// (00 §6, one-type-per-channel, mirroring ProjectDTO). Read-path DTOs leave
	// it empty.
	Status string `json:"status,omitempty"`
	// Order is the repository's dense index within its project's sidebar section.
	Order int `json:"order"`
}

// RepoDTOFrom maps a domain Repository onto the wire DTO. Icon precedence is
// resolved client-side (emoji > on-disk image > generated label/color); this
// converter only surfaces the proxy URL when an on-disk icon exists and passes
// the emoji through untouched.
func RepoDTOFrom(r domain.Repository) RepoDTO {
	avatarURL := ""
	if r.AvatarHasIcon {
		// The ?v=<AvatarVersion> query param cache-busts the otherwise-stable
		// icon URL: uploads replace the bytes in place, and without a changing
		// URL the webview's image cache keeps serving the old icon.
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
		Order:         r.Order,
	}
}

// RepoDTOList converts a slice of domain Repositories into wire DTOs in sidebar
// order, returning a non-nil empty slice when the input is empty so the envelope
// carries [].
//
// The sort lives HERE, in the converter both the REST list handler and the WS
// snapshot go through, because those are the two answers to the same question
// and a client that got different orders from them would watch its sidebar
// reshuffle on every reconnect.
func RepoDTOList(repos []domain.Repository) []RepoDTO {
	dtos := make([]RepoDTO, 0, len(repos))
	for _, r := range repos {
		dtos = append(dtos, RepoDTOFrom(r))
	}
	slices.SortFunc(dtos, compareRepoDTOs)
	return dtos
}

// compareRepoDTOs orders repositories by their dense index within a project,
// then by id. Order is only meaningful within one project, so a cross-project
// list is every project's sequence interleaved; the client groups by projectId
// and reads each group in this order.
func compareRepoDTOs(
	a RepoDTO,
	b RepoDTO,
) int {
	if a.Order != b.Order {
		return a.Order - b.Order
	}
	return strings.Compare(a.ID, b.ID)
}
