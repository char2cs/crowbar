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

// BranchChat is a lightweight projection of a Chat (01) surfaced read-only
// inside the branch review panel (09 §2). Title and Age are derived fields;
// IsActive reflects whether the underlying chat's agent is currently running.
type BranchChat struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Age      string `json:"age"`
	IsActive bool   `json:"isActive"`
}
