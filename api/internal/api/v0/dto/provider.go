package dto

import (
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

// PRInfoDTO is the wire shape of the resolved pull-request state for a branch:
// its number, status ("open" | "merged" | "closed"), URL, title, and target
// branch (02 §2.6).
type PRInfoDTO struct {
	Number       int    `json:"number"`
	Status       string `json:"status"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	TargetBranch string `json:"targetBranch"`
}

// ProviderStateDTO is the wire shape of a provider poll snapshot: whether the
// workspace branch is protected and the most relevant pull request, nil when
// the branch has none.
type ProviderStateDTO struct {
	Protected bool       `json:"protected"`
	PR        *PRInfoDTO `json:"pr,omitempty"`
}

// ProviderStateDTOFrom converts an engine ProviderState into its wire DTO,
// preserving the nil PR when the branch has no associated pull request.
func ProviderStateDTOFrom(
	state providertypes.ProviderState,
) ProviderStateDTO {
	dto := ProviderStateDTO{
		Protected: state.Protected,
	}
	if state.PR == nil {
		return dto
	}
	dto.PR = &PRInfoDTO{
		Number:       state.PR.Number,
		Status:       state.PR.Status,
		URL:          state.PR.URL,
		Title:        state.PR.Title,
		TargetBranch: state.PR.TargetBranch,
	}
	return dto
}

// ProtectedBranchList returns a non-nil copy of the protected branch names so
// the envelope carries [] rather than null when the repo has none.
func ProtectedBranchList(
	branches []string,
) []string {
	out := make([]string, 0, len(branches))
	out = append(out, branches...)
	return out
}
