package workspace

import "github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"

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
	OwningChats      = hierarchy.OwningChats
	ImportedBranch   = hierarchy.ImportedBranch
	NodeCreator      = hierarchy.NodeCreator
)

var WithTerminalReaper = hierarchy.WithTerminalReaper

var WithNodes = hierarchy.WithNodes

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
