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
// sidebar row. Mutated only through Asynx commands.
type Workspace struct {
	ID           string `json:"id"`
	RepoID       string `json:"repoId"`
	ProjectID    string `json:"projectId"`
	Branch       string `json:"branch"`
	WorktreePath string `json:"worktreePath"`
	ForkPointSha string `json:"forkPointSha"`
	// ParentID is the fork parent's workspace id, written at creation and afterwards only by the explicit
	// git-lineage reparent. Chat tree moves never re-project it, so a sidebar drag changes organisation, not
	// lineage.
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
	// Working is a derived, never-persisted overlay; with agent runs removed it is always false.
	Working bool `json:"working"`
	// IsDefault marks the repo's main worktree (the imported folder). It is served like any workspace; the
	// frontend opens it from the repo header by its real id.
	IsDefault bool `json:"isDefault,omitempty"`
	// Kind distinguishes git-worktree workspaces ("git", default) from the
	// project-level home workspace ("home"). Old persisted records without this
	// field replay as WorkspaceKindGit.
	Kind WorkspaceKind `json:"kind,omitempty"`
	// HeldByPath is the worktree directory holding this placeholder's branch (a locked row with no
	// WorktreePath), the one durable signal for the placeholder reason; Retry clears it.
	HeldByPath string `json:"heldByPath,omitempty"`
	// LockOverride is the user's answer to "is this locked", outranking the provider's protected flag. nil
	// means no opinion (protected => locked); a third state is needed because the provider re-answers every
	// poll, and both *true and *false must survive those polls.
	LockOverride *bool `json:"lockOverride,omitempty"`
	// CreatedBranch records that Crowbar created Branch for this workspace: the only licence a teardown has
	// to delete it. Older rows replay false, erring on the side of keeping a branch.
	CreatedBranch bool `json:"createdBranch,omitempty"`
	// Provisioning is what stands behind WorktreePath (see its type). A row
	// written before the field existed is given one by BackfillProvisioning.
	Provisioning WorkspaceProvisioning `json:"provisioning,omitempty"`
}

// RendersAsBranch reports whether w draws its OWN sidebar row (a locked branch with its own workspace
// Node), unlike an unlocked fork drawn through its owning chat. Default and home workspaces are left out
// on purpose: widening it risks a ghost row against the owning chat.
func (w Workspace) RendersAsBranch() bool {
	return w.Status == WorkspaceStatusLocked
}
