package dto

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type ChatWorktreeDTO struct {
	Branch          string                       `json:"branch"`
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
	ForkPointSha    string                       `json:"forkPointSha,omitempty"`
	ParentID        string                       `json:"parentId,omitempty"`
	FolderID        string                       `json:"folderId"`
	Order           int                          `json:"order"`
	CreatedAt       time.Time                    `json:"createdAt"`
	OwningChatID    string                       `json:"owningChatId"`
}

func ChatWorktreeFrom(
	w WorkspaceDTO,
) *ChatWorktreeDTO {
	return &ChatWorktreeDTO{
		Branch:          w.Branch,
		Status:          w.Status,
		Working:         w.Working,
		IsDefault:       w.IsDefault,
		Added:           w.Added,
		Deleted:         w.Deleted,
		MergeStrategy:   w.MergeStrategy,
		CanMergeLocally: w.CanMergeLocally,
		MergeConflicts:  w.MergeConflicts,
		ParentBranch:    w.ParentBranch,
		PRUrl:           w.PRUrl,
		PRTitle:         w.PRTitle,
		PRTargetBranch:  w.PRTargetBranch,
		LocalPath:       w.LocalPath,
		HeldByPath:      w.HeldByPath,
		Provisioning:    w.Provisioning,
		ForkPointSha:    w.ForkPointSha,
		ParentID:        w.ParentID,
		OwningChatID:    w.OwningChatID,
		FolderID:        w.FolderID,
		Order:           w.Order,
		CreatedAt:       w.CreatedAt,
	}
}
