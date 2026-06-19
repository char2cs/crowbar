package dto

import (
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// WorkspaceDTO is the wire shape of a Workspace: the git-worktree aggregate that
// backs a sidebar row (00 §5.3). It carries the live-badge fields the frontend
// renders — diff counts, conflict and lock state, status, merge strategy, and
// the pull-request summary — without leaking persistence-only fields.
type WorkspaceDTO struct {
	ID             string                  `json:"id"`
	WorktreePath   string                  `json:"worktreePath,omitempty"`
	RepoID         string                  `json:"repoId"`
	ProjectID      string                  `json:"projectId"`
	Branch         string                  `json:"branch"`
	ParentID       string                  `json:"parentId,omitempty"`
	ForkPointSha   string                  `json:"forkPointSha,omitempty"`
	Status         domain.WorkspaceStatus  `json:"status,omitempty"`
	Locked         bool                    `json:"locked"`
	HasConflicts   bool                    `json:"hasConflicts"`
	Added          int                     `json:"added"`
	Deleted        int                     `json:"deleted"`
	MergeStrategy  gitdomain.MergeStrategy `json:"mergeStrategy"`
	PRUrl          string                  `json:"prUrl,omitempty"`
	PRTitle        string                  `json:"prTitle,omitempty"`
	PRTargetBranch string                  `json:"prTargetBranch,omitempty"`
	Working        bool                    `json:"working"`
	PendingMerge   *gitdomain.PendingMerge `json:"pendingMerge,omitempty"`
}

// WorkspaceDTOFrom converts a domain Workspace into its wire DTO.
func WorkspaceDTOFrom(
	w domain.Workspace,
) WorkspaceDTO {
	return WorkspaceDTO{
		ID:             w.ID,
		WorktreePath:   w.WorktreePath,
		RepoID:         w.RepoID,
		ProjectID:      w.ProjectID,
		Branch:         w.Branch,
		ParentID:       w.ParentID,
		ForkPointSha:   w.ForkPointSha,
		Status:         w.Status,
		Locked:         w.Locked,
		HasConflicts:   w.HasConflicts,
		Added:          w.Added,
		Deleted:        w.Deleted,
		MergeStrategy:  w.MergeStrategy,
		PRUrl:          w.PRUrl,
		PRTitle:        w.PRTitle,
		PRTargetBranch: w.PRTargetBranch,
		Working:        w.Working,
		PendingMerge:   w.PendingMerge,
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
