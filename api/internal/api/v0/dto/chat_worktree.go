package dto

import (
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// ChatWorktreeDTO is the git half of a chat that OWNS a worktree: the branch it
// cut, what is uncommitted in it, where it stands against its parent, and what
// its pull request says (spec §5, "the Chat DTO gains the git fields
// Workspace's own DTO currently carries").
//
// It rides AgentChatDTO as a POINTER and is absent for a chat that owns no
// worktree, because absence is the honest encoding and a zero value would not
// be: a bubble does not have a branch named "" with nothing added and nothing
// deleted, it has no branch at all, and a client that could not tell those
// apart would draw an empty diff badge on every conversation in the panel.
//
// The workspace's own id is deliberately NOT here. It is already on the chat
// (AgentChatDTO.WorkspaceID), and duplicating it inside the nested object would
// give a client two places to read the same fact from — one of which it could
// address a request to, which is exactly what law 1 exists to prevent.
type ChatWorktreeDTO struct {
	Branch string `json:"branch"`
	// Status is the wire status WorkspaceDTO computes, conflict overlay and all
	// (see effectiveStatus) — never the raw persisted one, or a chat and its
	// workspace would disagree about the same branch.
	Status    domain.WorkspaceStatus `json:"status,omitempty"`
	LastError string                 `json:"lastError,omitempty"`
	// Working is the WORKTREE's own busy state — a long-running git operation on
	// it — and is a different fact from AgentChatDTO.Working, which is the chat's
	// folded turn state. A row can be one without the other (a sync running under
	// an idle conversation, a turn in flight over a quiet worktree), so they are
	// two fields rather than one, exactly as they are on the two aggregates.
	Working bool `json:"working"`
	// IsDefault marks the repo's own default-branch worktree, the row a client
	// draws as the repo header rather than as a child. It is carried because
	// WorkspaceDTO carries it today and this DTO must be able to replace that one
	// without a client losing the distinction; spec §2 deletes the field from the
	// domain, and it drops from here in the same breath.
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
	LocalPath  string `json:"localPath,omitempty"`
	HeldByPath string `json:"heldByPath,omitempty"`
	// ForkPointSha and ParentID describe the worktree's GIT lineage — where it was
	// cut from, and the workspace it was cut off. ParentID is a workspace id and
	// not a chat id on purpose: it is the fork/PR lineage domain.Workspace has
	// always written once at creation, a different field from the chat's own
	// ParentID (sidebar placement), and §7.6 records why the two must not be
	// conflated. A client resolves it the way the workspace list already does.
	ForkPointSha string `json:"forkPointSha,omitempty"`
	ParentID     string `json:"parentId,omitempty"`
	// OwningChatID names which chat OWNS this worktree, and it is the one field
	// here that is not a fact about git.
	//
	// It exists because a worktree is many-chats-to-one: a thread carries its
	// parent's workspace id, so several rows in one list legitimately describe
	// the same branch, and every one of them gets these fields (law 5 — shared
	// state is shared, and a sibling asking about the branch gets the same
	// answer). A client assembling a per-WORKTREE view therefore needs to know
	// which of those rows is the one the worktree is addressed by, and deriving
	// it client-side would be a second, independently-drifting copy of
	// ResolveOwningChat's branch-preferring rule. So the daemon says.
	//
	// It is the SAME answer WorkspaceDTO.OwningChatID carries, resolved by the
	// same call, which is what lets a client keep one identity for a worktree
	// across both surfaces.
	OwningChatID string `json:"owningChatId"`
}

// ChatWorktreeFrom projects a workspace's own wire DTO down to the git half a
// chat carries.
//
// It takes the BUILT WorkspaceDTO rather than a domain.Workspace plus an
// eligibility, and that is the whole point of its shape: every field here is
// one WorkspaceDTOFrom already resolved — the conflict-overlaid Status, the
// merge-eligibility overlay, the worktree path — so the chat-scoped answer and
// the workspace-scoped answer are the same bytes by construction and cannot
// drift as either side gains a field. The two live REST paths and the live WS
// push all funnel through here for exactly that reason.
func ChatWorktreeFrom(
	w WorkspaceDTO,
) *ChatWorktreeDTO {
	return &ChatWorktreeDTO{
		Branch:          w.Branch,
		Status:          w.Status,
		LastError:       w.LastError,
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
		ForkPointSha:    w.ForkPointSha,
		ParentID:        w.ParentID,
		OwningChatID:    w.OwningChatID,
	}
}
