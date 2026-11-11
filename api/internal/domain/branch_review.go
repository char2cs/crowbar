package domain

import gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"

// BranchReview is the composite review read model the usecase assembles from the git
// engine and the ReviewThread repo; it is never stored as a single entity.
type BranchReview struct {
	Description   string                  `json:"description"`
	MergeStrategy gitdomain.MergeStrategy `json:"mergeStrategy"`
	Threads       []ReviewThread          `json:"threads"`
}
