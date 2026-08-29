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
	ID           string `json:"id"`
	RepoID       string `json:"repoId"`
	ProjectID    string `json:"projectId"`
	Branch       string `json:"branch"`
	WorktreePath string `json:"worktreePath"`
	ForkPointSha string `json:"forkPointSha"`
	// ParentID is the fork parent's workspace id. It is written at CREATION —
	// where it is resolved once from the sidebar forest's fork-parent walk
	// (usecases/chat/internal/tree.ForkParentID) — and afterwards only by the
	// explicit git-lineage reparent route (POST .../workspaces/:wsId/reparent,
	// the workspace repository's Reparent command, which RebaseOntoParent shares).
	// It is NOT re-projected when a row moves in the chat tree: tree.Move and
	// tree.PlaceChat write domain.Chat.ParentID and nothing else, so a sidebar
	// drag changes organisation without changing git lineage. The three consumers
	// that resolve it back to a workspace — merge eligibility, the diff base, the
	// reparent leaf guard — read it exactly as they always have.
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
	// LockOverride is the user's own answer to "is this workspace locked",
	// outranking the provider's protected flag. nil — the default, and what every
	// row replays as — means "no opinion": the branch is locked iff the provider
	// says it is protected, which is the automatic locking that has always been
	// here and stays exactly as it was.
	//
	// It has to be a THIRD state rather than a plain bool, because the provider
	// re-answers the protected question on every poll (see nextProviderStatus).
	// With only true/false there is no way to say "I have not chosen", so a
	// default-false would read as "the user unlocked this" and quietly strip the
	// lock off every protected branch on the next sync.
	//
	// *true locks a branch the provider does not protect; *false unlocks one it
	// does. Both survive provider polls, which is the entire point: a user who
	// unlocked main must not find it locked again a minute later.
	LockOverride *bool `json:"lockOverride,omitempty"`
}
