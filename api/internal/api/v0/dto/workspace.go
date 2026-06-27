package dto

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// WorkspaceDTO is the wire shape of a Workspace: the git-worktree aggregate that
// backs a sidebar row (00 §5.3). It carries the live-badge fields the frontend
// renders — diff counts, status, merge strategy, the merge-eligibility overlay,
// the last-error overlay, and the pull-request summary.
type WorkspaceDTO struct {
	ID              string                  `json:"id"`
	RepoID          string                  `json:"repoId"`
	ProjectID       string                  `json:"projectId"`
	Kind            domain.WorkspaceKind    `json:"kind,omitempty"`
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
	MergeConflicts  bool                    `json:"mergeConflicts"`
	ParentBranch    string                  `json:"parentBranch,omitempty"`
	PRUrl           string                  `json:"prUrl,omitempty"`
	PRTitle         string                  `json:"prTitle,omitempty"`
	PRTargetBranch  string                  `json:"prTargetBranch,omitempty"`
	// LocalPath is the on-disk worktree directory for this workspace. Clients use
	// it to construct absolute file paths (e.g. "Copy Path" in the file explorer).
	// Exposed here so workspace-creation details never bleed into client code.
	LocalPath string `json:"localPath,omitempty"`
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
		Kind:            w.Kind,
		Branch:          w.Branch,
		ParentID:        w.ParentID,
		ForkPointSha:    w.ForkPointSha,
		Status:          effectiveStatus(w.Status, elig.MergeConflicts),
		Working:         w.Working,
		LastError:       w.LastError,
		IsDefault:       w.IsDefault,
		Added:           w.Added,
		Deleted:         w.Deleted,
		MergeStrategy:   w.MergeStrategy,
		CanMergeLocally: elig.CanMergeLocally,
		MergeConflicts:  elig.MergeConflicts,
		ParentBranch:    elig.ParentBranch,
		PRUrl:           w.PRUrl,
		PRTitle:         w.PRTitle,
		PRTargetBranch:  w.PRTargetBranch,
		LocalPath:       w.WorktreePath,
	}
}

// effectiveStatus folds the predicted "conflicts with parent" signal into the
// wire status: a branch that would conflict with its parent — whether from a
// reparent's failed rebase or the merge-tree prediction — is surfaced as
// pr-conflicts, the single conflict state the UI resolves on. A terminal/locked
// status takes precedence. The persisted aggregate status is unchanged; this is
// a read-time overlay, like the merge-eligibility overlay above.
func effectiveStatus(base domain.WorkspaceStatus, mergeConflicts bool) domain.WorkspaceStatus {
	if mergeConflicts &&
		base != domain.WorkspaceStatusDeleted &&
		base != domain.WorkspaceStatusLocked {
		return domain.WorkspaceStatusPRConflicts
	}
	return base
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
