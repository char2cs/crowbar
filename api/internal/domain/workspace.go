package domain

import (
	"time"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// WorkspaceKind distinguishes git-worktree workspaces from the project-level
// home workspace which has no branch and no git operations.
type WorkspaceKind string

const (
	WorkspaceKindGit  WorkspaceKind = "git"
	WorkspaceKindHome WorkspaceKind = "home"
)

// Workspace is the git-worktree aggregate; the single source of truth for the
// sidebar row (00 §5.3). Mutated only through Asynx commands.
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
	// Working is a derived, non-persisted overlay (00 §6.1, 03 §7): true iff the
	// workspace has live work in progress. It is computed at broadcast time and is
	// never written by any command. With the agent-run concept removed it is
	// always false in scope.
	Working bool `json:"working"`
	// LastError carries the message from the most recent failed background
	// mutation (00 §4): create/sync/merge/reparent set it on failure so the wire
	// DTO can surface the error against the entity. Empty when the last mutation
	// succeeded.
	LastError string `json:"lastError,omitempty"`
	// IsDefault marks the workspace that represents the repo's main worktree (the
	// on-disk folder the user originally imported). It is served on every list and
	// stream like any other workspace; the frontend pulls it out of the sidebar
	// tree and opens it from the repo header by its real id (there is no "default"
	// wsId alias).
	IsDefault bool `json:"isDefault,omitempty"`
	// Kind distinguishes git-worktree workspaces ("git", default) from the
	// project-level home workspace ("home"). Old persisted records without this
	// field replay as WorkspaceKindGit.
	Kind WorkspaceKind `json:"kind,omitempty"`
	// HeldByPath is the worktree directory currently holding this workspace's
	// branch, set only on a PLACEHOLDER (a locked row with an empty WorktreePath)
	// that could not get a managed worktree because a live worktree — the repo
	// home or an external checkout — holds the branch. It is the single durable
	// signal from which the frontend reconstructs the placeholder reason; a
	// successful Retry clears it. Empty on every healthy workspace (00 §4, spec §4).
	HeldByPath string `json:"heldByPath,omitempty"`
	// FolderID is the sidebar Folder this workspace has been filed under, or ""
	// for the repo root. It is DELIBERATELY not ParentID: that field is the fork
	// lineage, and three things resolve it back to a workspace (merge
	// eligibility, the diff base, the reparent leaf guard), so a folder id in
	// there is a silent corruption rather than an error.
	//
	// It is meaningful only on a FORK ROOT (ParentID == ""). A workspace with a
	// fork parent renders under that parent and inherits its folder from its fork
	// ancestor, so filing one away separately would split the fork chain —
	// refused server-side, and cleared by Reparent. Old persisted records without
	// this field replay as "" (the read model is a JSON blob), exactly as Kind
	// documents above.
	FolderID string `json:"folderId,omitempty"`
	// Order is this row's dense index within its sibling space — the fork
	// parent's children, or the folder/repo root it is filed under. Folders share
	// that space, so both kinds sort on this one field. Rows a user has never
	// ordered all carry 0 and fall back to the CreatedAt tiebreak, which is
	// creation order; the first drag at a level renumbers it densely from
	// whatever was on screen.
	Order int `json:"order"`
}
