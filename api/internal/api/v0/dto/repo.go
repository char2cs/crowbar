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
	// Status is the tombstone marker on the Repo broadcast channel: "deleted" on a
	// removal frame so the client cache drops it; "" otherwise.
	Status string `json:"status,omitempty"`
	// Order is the repository's dense index within its project's sidebar section.
	Order int `json:"order"`
	// FolderID is the project-home folder this repo's entry is filed under, ""
	// for the project's home root.
	FolderID string `json:"folderId,omitempty"`
	// Deleting is the recorded delete intent. Announced before the cascade, so
	// clients stop reading a repo whose chats are about to be tombstoned.
	Deleting bool `json:"deleting,omitempty"`
}

// RepoPlacement is a repo's sidebar position, read off its Node{Kind: NodeKindRepo}
// row rather than domain.Repository. The zero value (root, order 0) means no Node row yet.
type RepoPlacement struct {
	FolderID string
	Order    int
}

// RepoDTOFrom maps a repository plus its Node-sourced placement onto the wire DTO.
// Icon precedence (emoji > on-disk image > generated) is resolved client-side.
func RepoDTOFrom(r domain.Repository, placement RepoPlacement) RepoDTO {
	avatarURL := ""
	if r.AvatarHasIcon {
		// ?v=<AvatarVersion> cache-busts the stable icon URL: uploads replace the bytes
		// in place, and the webview's image cache would keep serving the old icon.
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

// RepoDTOList converts repos into a non-nil DTO slice in sidebar order, placed by id
// from placements. The sort lives here because REST and the WS snapshot both use it,
// and differing orders would reshuffle the sidebar on every reconnect.
func RepoDTOList(repos []domain.Repository, placements map[string]RepoPlacement) []RepoDTO {
	dtos := make([]RepoDTO, 0, len(repos))
	for _, r := range repos {
		dtos = append(dtos, RepoDTOFrom(r, placements[r.ID]))
	}
	slices.SortFunc(dtos, compareRepoDTOs)
	return dtos
}

// compareRepoDTOs orders repositories by dense index, then id. Order is meaningful only
// within a project, so the client groups a cross-project list by projectId.
func compareRepoDTOs(
	a RepoDTO,
	b RepoDTO,
) int {
	if a.Order != b.Order {
		return a.Order - b.Order
	}
	return strings.Compare(a.ID, b.ID)
}
