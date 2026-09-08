package domain

// NodeKind marks which entity a Node's position row stands in for: a real
// conversation, a folder, or — referenced directly, never through a proxy chat
// — a workspace (a locked branch, a repo's own checkout, or project home) or a
// repo import (2026-09-08 sidebar-placement-unification design §2.1).
type NodeKind string

const (
	NodeKindChat      NodeKind = "chat"
	NodeKindFolder    NodeKind = "folder"
	NodeKindWorkspace NodeKind = "workspace"
	NodeKindRepo      NodeKind = "repo"
)

// Node is the ONE aggregate that owns every sidebar row's position, at every
// tree level — chat, folder, workspace and repo rows all share this single
// sibling-order space, replacing the scattered ParentID/Order fields today's
// separate aggregates each kept their own copy of (the divergence that let a
// dragged repo silently snap back to position 0). Mutated only through asynx
// commands (api/internal/app/repositories/node), mirroring domain.Chat.
//
// ID reuses the referenced entity's own id (already a globally unique UUID per
// kind) — no third id space to keep in sync.
type Node struct {
	ID string `json:"id"`
	// Kind is fixed at creation: a row's identity never changes kind by being
	// moved. See NodeKind.
	Kind NodeKind `json:"kind"`
	// ParentID is another Node's id, or "" for that tree's own root — organised
	// alongside SetPlacement/SetOrder's own docs (internal/commands), which own
	// the write-time rules for what may move where.
	ParentID string `json:"parentId,omitempty"`
	// Order is this row's dense index within ParentID's sibling space, shared by
	// every kind: a densify pass merges chat, folder, workspace and repo rows
	// under one parent and sorts them all on this one field.
	Order int `json:"order"`
}

// FileNodeType distinguishes files from directories in the tree (05 §2).
type FileNodeType string

const (
	FileNodeTypeFile      FileNodeType = "file"
	FileNodeTypeDirectory FileNodeType = "directory"
)

// FileNodeGitStatus is the git decoration on a file tree node (05 §2).
// Conflicted files from git are collapsed to Modified for the tree view.
type FileNodeGitStatus string

const (
	FileNodeGitStatusModified  FileNodeGitStatus = "modified"
	FileNodeGitStatusAdded     FileNodeGitStatus = "added"
	FileNodeGitStatusDeleted   FileNodeGitStatus = "deleted"
	FileNodeGitStatusUntracked FileNodeGitStatus = "untracked"
	FileNodeGitStatusRenamed   FileNodeGitStatus = "renamed"
)

// FileNode is a single entry in the lazy file tree (05 §2).
type FileNode struct {
	Name      string             `json:"name"`
	Path      string             `json:"path"`
	Type      FileNodeType       `json:"type"`
	Children  []FileNode         `json:"children,omitempty"`
	GitStatus *FileNodeGitStatus `json:"gitStatus,omitempty"`
}
