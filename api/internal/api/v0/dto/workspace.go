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

// WorkspaceDTO is the wire shape of a Workspace, the git-worktree aggregate behind a
// sidebar row, with the badge fields the client renders and the read-time overlays.
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
	// LocalPath is the on-disk worktree directory, from which clients build absolute
	// file paths (e.g. "Copy Path").
	LocalPath string `json:"localPath,omitempty"`
	// HeldByPath is the directory holding this branch when the workspace is a placeholder
	// (empty LocalPath); the client builds the "checked out elsewhere" reason from it.
	HeldByPath string `json:"heldByPath,omitempty"`
	// Provisioning says what stands behind LocalPath: "provisioned",
	// "placeholder" (no checkout yet; nothing may fork from or rebase onto it)
	// or "shared" (the user's own checkout). Clients read it, never the path.
	Provisioning domain.WorkspaceProvisioning `json:"provisioning"`
	// CreatedAt is the order the list sorts by, so a client merging its cache gets a
	// stable creation order for rows FolderID/Order do not place.
	CreatedAt time.Time `json:"createdAt"`
	// OwningChatID points at the chat row this workspace owns, resolved by the chat
	// tree's branch-preferring rule. It is a pointer, not an embedded copy, because a
	// fork's owning chat is also rendered as its own conversation.
	OwningChatID string `json:"owningChatId"`
	// FolderID and Order are the sidebar placement PlaceWorkspace writes, separate from
	// ParentID (the fork parent) so a fork doesn't move when its lineage changes. Never
	// omitted: "" is the repo root and 0 the first slot.
	FolderID string `json:"folderId"`
	Order    int    `json:"order"`
}

// WorkspacePlacementReader resolves a workspace's sidebar FolderID/Order from its
// Node{Kind:workspace} row, kept narrow so this package needn't import the tree usecase.
// A workspace with no Node row yet resolves to "" / 0 rather than failing the row.
type WorkspacePlacementReader interface {
	Placement(ctx context.Context, workspaceID string) (folderID string, order int)
}

// WorkspaceDTOFrom converts a Workspace into its wire DTO. The caller resolves the merge
// eligibility over the repo's siblings, so the converter does no repo-wide read; a nil
// placement leaves FolderID/Order at "" / 0.
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

// effectiveStatus surfaces a predicted conflict with the parent as pr-conflicts, the one
// conflict state the UI resolves on; deleted and locked take precedence. It is a
// read-time overlay: the persisted status is unchanged.
func effectiveStatus(base domain.WorkspaceStatus, mergeConflicts bool) domain.WorkspaceStatus {
	if mergeConflicts &&
		base != domain.WorkspaceStatusDeleted &&
		base != domain.WorkspaceStatusLocked {
		return domain.WorkspaceStatusPRConflicts
	}
	return base
}

// WorkspaceDTOList converts workspaces into a non-nil, sorted DTO slice via eligFn,
// owningChatIDFn and placement. The sort lives here because REST and the WS snapshot
// both use it, and differing orders would reshuffle the sidebar on every reconnect.
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

// compareWorkspaceDTOs orders workspaces by creation time, then by id.
func compareWorkspaceDTOs(
	a WorkspaceDTO,
	b WorkspaceDTO,
) int {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Compare(b.CreatedAt)
	}
	return strings.Compare(a.ID, b.ID)
}
