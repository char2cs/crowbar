package dto

import (
	"strconv"

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
	}
}

func RepoDTOList(repos []domain.Repository) []RepoDTO {
	dtos := make([]RepoDTO, 0, len(repos))
	for _, r := range repos {
		dtos = append(dtos, RepoDTOFrom(r))
	}
	return dtos
}
