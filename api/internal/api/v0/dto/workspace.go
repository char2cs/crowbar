package dto

import (
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// WorkspaceDTO is the wire shape of a Workspace: the git-worktree aggregate that
// backs a sidebar row (00 §5.3). It carries the live-badge fields the frontend
// renders — diff counts, status, merge strategy, the merge-eligibility overlay,
// the last-error overlay, and the pull-request summary — without leaking
// persistence-only fields (the on-disk worktree path stays server-side).
type WorkspaceDTO struct {
	ID              string                  `json:"id"`
	RepoID          string                  `json:"repoId"`
	ProjectID       string                  `json:"projectId"`
	Branch          string                  `json:"branch"`
	ParentID        string                  `json:"parentId,omitempty"`
	ForkPointSha    string                  `json:"forkPointSha,omitempty"`
	Status          domain.WorkspaceStatus  `json:"status,omitempty"`
	Working         bool                    `json:"working"`
	LastError       string                  `json:"lastError,omitempty"`
	Added           int                     `json:"added"`
	Deleted         int                     `json:"deleted"`
	MergeStrategy   gitdomain.MergeStrategy `json:"mergeStrategy"`
	CanMergeLocally bool                    `json:"canMergeLocally"`
	ParentBranch    string                  `json:"parentBranch,omitempty"`
	PRUrl           string                  `json:"prUrl,omitempty"`
	PRTitle         string                  `json:"prTitle,omitempty"`
	PRTargetBranch  string                  `json:"prTargetBranch,omitempty"`
}

// WorkspaceDTOFrom converts a domain Workspace into its wire DTO. The merge
// eligibility overlay (CanMergeLocally/ParentBranch) requires the workspace's
// sibling set and is populated by the eligibility-aware callers wired in W8;
// here it maps to its zero value.
func WorkspaceDTOFrom(
	w domain.Workspace,
) WorkspaceDTO {
	return WorkspaceDTO{
		ID:             w.ID,
		RepoID:         w.RepoID,
		ProjectID:      w.ProjectID,
		Branch:         w.Branch,
		ParentID:       w.ParentID,
		ForkPointSha:   w.ForkPointSha,
		Status:         w.Status,
		Working:        w.Working,
		LastError:      w.LastError,
		Added:          w.Added,
		Deleted:        w.Deleted,
		MergeStrategy:  w.MergeStrategy,
		PRUrl:          w.PRUrl,
		PRTitle:        w.PRTitle,
		PRTargetBranch: w.PRTargetBranch,
	}
}

// WorkspaceDTOList converts a slice of domain Workspaces into wire DTOs,
// returning a non-nil empty slice when the input is empty so the envelope
// carries [].
func WorkspaceDTOList(
	workspaces []domain.Workspace,
) []WorkspaceDTO {
	dtos := make([]WorkspaceDTO, 0, len(workspaces))
	for _, w := range workspaces {
		dtos = append(dtos, WorkspaceDTOFrom(w))
	}
	return dtos
}
