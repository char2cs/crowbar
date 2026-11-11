package workspace

import "github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"

// CreateChildInput, ImportInput and MergeResult are the hierarchy usecase's
// own value types, re-exported here so a caller of this package's public face
// never has to import internal/hierarchy directly.
type (
	CreateChildInput = hierarchy.CreateChildInput
	ImportInput      = hierarchy.ImportInput
	ImportBranch     = hierarchy.ImportBranch
	ImportProgress   = hierarchy.ImportProgress
	ImportOutcome    = hierarchy.ImportOutcome
	MergeResult      = hierarchy.MergeResult
	Option           = hierarchy.Option
	TerminalReaper   = hierarchy.TerminalReaper
	ChatWorkObserver = hierarchy.ChatWorkObserver
	// OwningChats is the chat-side surface every workspace this usecase creates
	// is born under, and ImportedBranch is the one branch an import describes to
	// it. Re-exported so the composition root can name what it must satisfy.
	OwningChats    = hierarchy.OwningChats
	ImportedBranch = hierarchy.ImportedBranch
	// NodeCreator is the narrow Node surface WithNodes wires — re-exported for
	// the same reason as the value types above.
	NodeCreator = hierarchy.NodeCreator
)

// WithTerminalReaper wires the terminal engine so cascade delete can kill a
// workspace's live PTY sessions. Without it, terminal cleanup is skipped.
var WithTerminalReaper = hierarchy.WithTerminalReaper

// WithNodes wires the Node surface every workspace this usecase creates mints
// its own position row through; see hierarchy.WithNodes.
var WithNodes = hierarchy.WithNodes

// The hierarchy usecase's sentinel errors, re-exported here for the same
// reason as the value types above.
var (
	ErrParentLocked                 = hierarchy.ErrParentLocked
	ErrRebaseNonLeaf                = hierarchy.ErrRebaseNonLeaf
	ErrChildHasChildren             = hierarchy.ErrChildHasChildren
	ErrCrossRepoWorktreeMove        = hierarchy.ErrCrossRepoWorktreeMove
	ErrSelfParent                   = hierarchy.ErrSelfParent
	ErrWorkspaceLocked              = hierarchy.ErrWorkspaceLocked
	ErrWorkspaceIsDefault           = hierarchy.ErrWorkspaceIsDefault
	ErrWorkspaceIsHome              = hierarchy.ErrWorkspaceIsHome
	ErrBranchWorkspaceExists        = hierarchy.ErrBranchWorkspaceExists
	ErrParentUnprovisioned          = hierarchy.ErrParentUnprovisioned
	ErrRenameUnmanagedWorkspace     = hierarchy.ErrRenameUnmanagedWorkspace
	ErrRenameTargetExists           = hierarchy.ErrRenameTargetExists
	ErrBranchStillHeld              = hierarchy.ErrBranchStillHeld
	ErrBranchHeldByManagedWorkspace = hierarchy.ErrBranchHeldByManagedWorkspace
	ErrWorkspaceWorking             = hierarchy.ErrWorkspaceWorking
	ErrNoOwningChats                = hierarchy.ErrNoOwningChats
)
