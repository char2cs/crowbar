package domain

import "time"

// Workspace is the git-worktree aggregate; the single source of truth for the
// sidebar row (00 §5.3). Mutated only through Asynx commands.
type Workspace struct {
	ID             string          `json:"id"`
	RepoID         string          `json:"repoId"`
	ProjectID      string          `json:"projectId"`
	Branch         string          `json:"branch"`
	WorktreePath   string          `json:"worktreePath"`
	ForkPointSha   string          `json:"forkPointSha"`
	ParentID       string          `json:"parentId,omitempty"`
	Status         WorkspaceStatus `json:"status,omitempty"`
	Locked         bool            `json:"locked"`
	HasConflicts   bool            `json:"hasConflicts"`
	MergeStrategy  MergeStrategy   `json:"mergeStrategy"`
	PendingMerge   *PendingMerge   `json:"pendingMerge,omitempty"`
	Added          int             `json:"added"`
	Deleted        int             `json:"deleted"`
	PRUrl          string          `json:"prUrl,omitempty"`
	PRTitle        string          `json:"prTitle,omitempty"`
	PRTargetBranch string          `json:"prTargetBranch,omitempty"`
	LastActivity   time.Time       `json:"lastActivity"`
	CreatedAt      time.Time       `json:"createdAt"`
}
