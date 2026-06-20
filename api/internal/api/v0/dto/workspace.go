package dto

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
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
	IsDefault       bool                    `json:"isDefault,omitempty"`
	Added           int                     `json:"added"`
	Deleted         int                     `json:"deleted"`
	MergeStrategy   gitdomain.MergeStrategy `json:"mergeStrategy"`
	CanMergeLocally bool                    `json:"canMergeLocally"`
	ParentBranch    string                  `json:"parentBranch,omitempty"`
	PRUrl           string                  `json:"prUrl,omitempty"`
	PRTitle         string                  `json:"prTitle,omitempty"`
	PRTargetBranch  string                  `json:"prTargetBranch,omitempty"`
}

// WorkspaceDTOFrom converts a domain Workspace into its wire DTO, populating the
// merge-eligibility overlay (CanMergeLocally/ParentBranch) from the resolved
// eligibility the caller computed via MergeEligibilityFor over the repo-scoped
// sibling set. Resolving eligibility outside the converter keeps the sibling
// read off the broadcast hot path (spec §10).
func WorkspaceDTOFrom(
	w domain.Workspace,
	elig workspace.MergeEligibility,
) WorkspaceDTO {
	return WorkspaceDTO{
		ID:              w.ID,
		RepoID:          w.RepoID,
		ProjectID:       w.ProjectID,
		Branch:          w.Branch,
		ParentID:        w.ParentID,
		ForkPointSha:    w.ForkPointSha,
		Status:          w.Status,
		Working:         w.Working,
		LastError:       w.LastError,
		IsDefault:       w.IsDefault,
		Added:           w.Added,
		Deleted:         w.Deleted,
		MergeStrategy:   w.MergeStrategy,
		CanMergeLocally: elig.CanMergeLocally,
		ParentBranch:    elig.ParentBranch,
		PRUrl:           w.PRUrl,
		PRTitle:         w.PRTitle,
		PRTargetBranch:  w.PRTargetBranch,
	}
}

// WorkspaceDTOList converts a slice of domain Workspaces into wire DTOs,
// resolving each row's merge eligibility through eligFn (typically a closure
// over MergeEligibilityFor bound to the same sibling slice). It returns a
// non-nil empty slice when the input is empty so the envelope carries [].
func WorkspaceDTOList(
	workspaces []domain.Workspace,
	eligFn func(domain.Workspace) workspace.MergeEligibility,
) []WorkspaceDTO {
	dtos := make([]WorkspaceDTO, 0, len(workspaces))
	for _, w := range workspaces {
		dtos = append(dtos, WorkspaceDTOFrom(w, eligFn(w)))
	}
	return dtos
}
