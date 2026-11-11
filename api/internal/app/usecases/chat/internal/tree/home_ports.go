package tree

import (
	"context"
	"time"

	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

// The placement surface: Folders is identity, Nodes is position, split the same way dtree.Folder and
// dtree.Node are.

// Folders is the identity store for folders of every scope (home and repo); a folder's position lives
// on Nodes, never here.
type Folders interface {
	FindByKey(
		ctx context.Context,
		id string,
	) (*dtree.Folder, error)
	// FindAll answers every folder, filtered by scope in memory: RepoID == "" is the store's zero value
	// and cannot be expressed as a query filter.
	FindAll(
		ctx context.Context,
	) ([]dtree.Folder, error)
	Save(
		ctx context.Context,
		f dtree.Folder,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
}

// Nodes is the position store every chat, folder, repo and workspace anchor shares, so each level's
// siblings come from one Node read.
type Nodes interface {
	Create(
		ctx context.Context,
		id string,
		kind dtree.Kind,
		parentID string,
		order int,
	) (dtree.Node, error)
	GetNode(
		ctx context.Context,
		id string,
	) (dtree.Node, error)
	ListByParent(
		ctx context.Context,
		parentID string,
	) ([]dtree.Node, error)
	// ListAll is every node in one read, for the walks that would otherwise
	// read the table once per parent.
	ListAll(
		ctx context.Context,
	) ([]dtree.Node, error)
	SetOrder(
		ctx context.Context,
		id string,
		order int,
	) error
	SetPlacement(
		ctx context.Context,
		id string,
		parentID string,
		order int,
	) error
	// Forget purges a Node row outright: a folder's, when the folder is erased or its create unwound.
	Forget(
		ctx context.Context,
		id string,
	) error
}

// WorkspaceGitStatus is the tree's read port onto the workspace layer: the facts about a workspace
// (its repo, its default checkout, whether it draws a branch row) that decide where its rows sit.
type WorkspaceGitStatus interface {
	// RepoOf answers the repo a workspace belongs to: "" for the project-home workspace, a repo id
	// otherwise (domain.Workspace.RepoID).
	RepoOf(
		ctx context.Context,
		workspaceID string,
	) (repoID string, err error)
	// OutlivesItsChats reports whether workspaceID is never reaped with the
	// chats in it: a project's home, or its repo's default checkout.
	OutlivesItsChats(
		ctx context.Context,
		workspaceID string,
	) (bool, error)
	// DefaultWorkspaceOf answers repoID's default checkout — whose root chats
	// sit at the repo's root level beside its folders and locked branches.
	DefaultWorkspaceOf(
		ctx context.Context,
		repoID string,
	) (string, error)
	// Exists reports whether workspaceID names a live workspace — the probe
	// ensureWorkspaceAnchor needs before minting a legacy row's anchor Node.
	Exists(
		ctx context.Context,
		workspaceID string,
	) (bool, error)
	// Provisioned reports whether workspaceID has a worktree on disk; a placeholder has none, so a chat
	// created in it has nowhere to run. An unresolvable id answers true: a failed read is not evidence.
	Provisioned(
		ctx context.Context,
		workspaceID string,
	) (bool, error)
	// RepoIDsForHome answers every repo id of homeWorkspaceID's project, so a home-root densify never
	// renumbers another project's repo Node rows.
	RepoIDsForHome(
		ctx context.Context,
		homeWorkspaceID string,
	) (map[string]bool, error)
	// HomeOfRepo answers the home workspace of repoID's project, "" for an unknown repo; it tells which
	// home a legacy folder holding that repo's header belongs to.
	HomeOfRepo(
		ctx context.Context,
		repoID string,
	) (string, error)
	// RendersAsBranch answers domain.Workspace.RendersAsBranch: a locked branch is a sidebar row of its
	// own, while an unlocked fork's anchor must not merge in beside the chat that already draws it.
	RendersAsBranch(
		ctx context.Context,
		workspaceID string,
	) (bool, error)
	// CreatedAtOf answers when workspaceID was created, the timestamp the sidebar breaks a branch row's
	// order ties on, so a tied level sorts the same on both sides.
	CreatedAtOf(
		ctx context.Context,
		workspaceID string,
	) (time.Time, error)
	// BranchRowsOf answers repoID's workspaces drawn as branch rows, default checkout excluded: members a
	// repo-root densify must count whether or not each has a Node row yet.
	BranchRowsOf(
		ctx context.Context,
		repoID string,
	) ([]string, error)
	// VisibleForkParent answers the workspace whose space a fork's row must stay inside: its ParentID,
	// or "" (the repo root) when that parent draws no row there (the default checkout is the root itself),
	// was never set, or no longer resolves.
	VisibleForkParent(
		ctx context.Context,
		workspaceID string,
	) (forkParentID string, err error)
}
