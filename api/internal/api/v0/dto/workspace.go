package dto

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type WorkspaceDTO struct {
	ID              string                       `json:"id"`
	RepoID          string                       `json:"repoId"`
	ProjectID       string                       `json:"projectId"`
	Kind            domain.WorkspaceKind         `json:"kind,omitempty"`
	Branch          string                       `json:"branch"`
	ParentID        string                       `json:"parentId,omitempty"`
	ForkPointSha    string                       `json:"forkPointSha,omitempty"`
	Status          domain.WorkspaceStatus       `json:"status,omitempty"`
	Working         bool                         `json:"working"`
	IsDefault       bool                         `json:"isDefault,omitempty"`
	Added           int                          `json:"added"`
	Deleted         int                          `json:"deleted"`
	MergeStrategy   gitdomain.MergeStrategy      `json:"mergeStrategy"`
	CanMergeLocally bool                         `json:"canMergeLocally"`
	MergeConflicts  bool                         `json:"mergeConflicts"`
	ParentBranch    string                       `json:"parentBranch,omitempty"`
	PRUrl           string                       `json:"prUrl,omitempty"`
	PRTitle         string                       `json:"prTitle,omitempty"`
	PRTargetBranch  string                       `json:"prTargetBranch,omitempty"`
	LocalPath       string                       `json:"localPath,omitempty"`
	HeldByPath      string                       `json:"heldByPath,omitempty"`
	Provisioning    domain.WorkspaceProvisioning `json:"provisioning"`
	CreatedAt       time.Time                    `json:"createdAt"`
	OwningChatID    string                       `json:"owningChatId"`
	FolderID        string                       `json:"folderId"`
	Order           int                          `json:"order"`
}

type WorkspacePlacementReader interface {
	Placement(ctx context.Context, workspaceID string) (folderID string, order int)
}

func WorkspaceDTOFrom(
	ctx context.Context,
	w domain.Workspace,
	elig workspace.MergeEligibility,
	owningChatID string,
	placement WorkspacePlacementReader,
) WorkspaceDTO {
	folderID, order := "", 0
	if placement != nil {
		folderID, order = placement.Placement(ctx, w.ID)
	}
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
		HeldByPath:      w.HeldByPath,
		Provisioning:    w.Provisioning,
		CreatedAt:       w.CreatedAt,
		OwningChatID:    owningChatID,
		FolderID:        folderID,
		Order:           order,
	}
}

func effectiveStatus(base domain.WorkspaceStatus, mergeConflicts bool) domain.WorkspaceStatus {
	if mergeConflicts &&
		base != domain.WorkspaceStatusDeleted &&
		base != domain.WorkspaceStatusLocked {
		return domain.WorkspaceStatusPRConflicts
	}
	return base
}

func WorkspaceDTOList(
	ctx context.Context,
	workspaces []domain.Workspace,
	eligFn func(domain.Workspace) workspace.MergeEligibility,
	owningChatIDFn func(domain.Workspace) string,
	placement WorkspacePlacementReader,
) []WorkspaceDTO {
	dtos := make([]WorkspaceDTO, 0, len(workspaces))
	for _, w := range workspaces {
		dtos = append(dtos, WorkspaceDTOFrom(ctx, w, eligFn(w), owningChatIDFn(w), placement))
	}
	slices.SortFunc(dtos, compareWorkspaceDTOs)
	return dtos
}

func compareWorkspaceDTOs(
	a WorkspaceDTO,
	b WorkspaceDTO,
) int {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Compare(b.CreatedAt)
	}
	return strings.Compare(a.ID, b.ID)
}
