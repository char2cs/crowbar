package domain

import gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"

type BranchReview struct {
	Description   string                  `json:"description"`
	MergeStrategy gitdomain.MergeStrategy `json:"mergeStrategy"`
	Threads       []ReviewThread          `json:"threads"`
}
