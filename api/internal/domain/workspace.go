package domain

import (
	"time"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type WorkspaceKind string

const (
	WorkspaceKindGit  WorkspaceKind = "git"
	WorkspaceKindHome WorkspaceKind = "home"
)

type Workspace struct {
	ID             string                  `json:"id"`
	RepoID         string                  `json:"repoId"`
	ProjectID      string                  `json:"projectId"`
	Branch         string                  `json:"branch"`
	WorktreePath   string                  `json:"worktreePath"`
	ForkPointSha   string                  `json:"forkPointSha"`
	ParentID       string                  `json:"parentId,omitempty"`
	Status         WorkspaceStatus         `json:"status,omitempty"`
	MergeStrategy  gitdomain.MergeStrategy `json:"mergeStrategy"`
	Added          int                     `json:"added"`
	Deleted        int                     `json:"deleted"`
	PRUrl          string                  `json:"prUrl,omitempty"`
	PRTitle        string                  `json:"prTitle,omitempty"`
	PRTargetBranch string                  `json:"prTargetBranch,omitempty"`
	LastActivity   time.Time               `json:"lastActivity"`
	CreatedAt      time.Time               `json:"createdAt"`
	Working        bool                    `json:"working"`
	IsDefault      bool                    `json:"isDefault,omitempty"`
	Kind           WorkspaceKind           `json:"kind,omitempty"`
	HeldByPath     string                  `json:"heldByPath,omitempty"`
	LockOverride   *bool                   `json:"lockOverride,omitempty"`
	CreatedBranch  bool                    `json:"createdBranch,omitempty"`
	Provisioning   WorkspaceProvisioning   `json:"provisioning,omitempty"`
}

func (w Workspace) RendersAsBranch() bool {
	return w.Status == WorkspaceStatusLocked
}
