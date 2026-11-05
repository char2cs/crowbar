package dto

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// ChatWorktreeDTO is the git half of a chat that owns a worktree. It is a pointer, absent
// for a chat with no worktree, so clients draw no empty diff badge. The workspace id is
// deliberately not repeated here: AgentChatDTO.WorkspaceID is its one home.
type ChatWorktreeDTO struct {
	Branch string `json:"branch"`
	// Status is WorkspaceDTO's computed status, conflict overlay included, never the raw
	// persisted one, so a chat and its workspace agree about the branch.
	Status domain.WorkspaceStatus `json:"status,omitempty"`
	// Working is the worktree's own busy state (a long git operation), independent of
	// AgentChatDTO.Working, the chat's turn state.
	Working bool `json:"working"`
	// IsDefault marks the repo's default-branch worktree, drawn as the repo header
	// rather than as a child.
	IsDefault bool `json:"isDefault,omitempty"`
	// Added and Deleted are the worktree's already-synced working-tree counts,
	// never a live git call — the same numbers the sidebar renders.
	Added           int                     `json:"added"`
	Deleted         int                     `json:"deleted"`
	MergeStrategy   gitdomain.MergeStrategy `json:"mergeStrategy"`
	CanMergeLocally bool                    `json:"canMergeLocally"`
	MergeConflicts  bool                    `json:"mergeConflicts"`
	ParentBranch    string                  `json:"parentBranch,omitempty"`
	PRUrl           string                  `json:"prUrl,omitempty"`
	PRTitle         string                  `json:"prTitle,omitempty"`
	PRTargetBranch  string                  `json:"prTargetBranch,omitempty"`
	// LocalPath is the on-disk worktree directory, and HeldByPath the directory
	// holding this branch when the worktree is a placeholder — see WorkspaceDTO's
	// own fields, which these are projected from.
	LocalPath    string                       `json:"localPath,omitempty"`
	HeldByPath   string                       `json:"heldByPath,omitempty"`
	Provisioning domain.WorkspaceProvisioning `json:"provisioning"`
	// ForkPointSha and ParentID are the worktree's git lineage. ParentID is a workspace
	// id written once at creation, not the chat's sidebar ParentID; the two must not be conflated.
	ForkPointSha string `json:"forkPointSha,omitempty"`
	ParentID     string `json:"parentId,omitempty"`
	// FolderID and Order are the workspace's sidebar placement, projected from
	// WorkspaceDTO. The web client derives a workspace's placement from this read only.
	FolderID string `json:"folderId"`
	Order    int    `json:"order"`
	// CreatedAt is the daemon's order tiebreak (tree.compareNodes) — carried so
	// a client draws an undragged level in the sequence a placement counts.
	CreatedAt time.Time `json:"createdAt"`
	// OwningChatID names which of the many chats sharing this worktree owns it, so a
	// client need not re-derive domain.ResolveOwningChat. It equals WorkspaceDTO.OwningChatID.
	OwningChatID string `json:"owningChatId"`
}

// ChatWorktreeFrom projects a built WorkspaceDTO down to the git half a chat carries.
// Taking the resolved DTO keeps the chat-scoped and workspace-scoped answers identical.
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
