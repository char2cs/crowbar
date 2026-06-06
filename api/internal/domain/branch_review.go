package domain

import gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"

// BranchReview is the composite read model returned by
// GET /v0/workspaces/:wsId/review (09 §2). It is assembled by the usecase
// from the git engine, the ReviewThread repo, and the Chat repo; it is never
// stored as a single entity.
type BranchReview struct {
	Description   string                  `json:"description"`
	MergeStrategy gitdomain.MergeStrategy `json:"mergeStrategy"`
	Diff          gitdomain.MultiFileDiff `json:"diff"`
	Threads       []ReviewThread          `json:"threads"`
	Conversations []BranchChat            `json:"conversations"`
}
