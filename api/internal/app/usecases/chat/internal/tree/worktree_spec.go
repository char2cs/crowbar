package tree

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/branchimport"
)

// WorktreeMode says how a new chat comes by the worktree it owns: none (a bubble or thread), a fresh fork,
// or an import of an existing branch. Every worktree is created through a chat, because a workspace with
// no owning chat cannot be addressed.
type WorktreeMode int

const (
	// WorktreeNone is a plain chat: a bubble, or a thread of another chat. It
	// owns no worktree and shares whatever its ancestry resolves to.
	WorktreeNone WorktreeMode = iota
	// WorktreeFork mints a fresh branch and worktree forked from the new chat's
	// resolved fork parent.
	WorktreeFork
	// WorktreeImport adopts a branch that already exists in the repository,
	// described by the spec's Import field.
	WorktreeImport
)

// WorktreeSpec is CreateChat's worktree half. Import is read only when Mode is WorktreeImport, so the zero
// value is a plain chat.
type WorktreeSpec struct {
	Mode WorktreeMode
	// Branch is read only when Mode is WorktreeFork; blank gets a server-generated name. Either way
	// the name is collision-checked.
	Branch string
	Import ImportSpec
}

// ImportSpec describes the existing branch a WorktreeImport create adopts. It is an alias so the shared
// test doubles in usecases/mocks, which cannot import this internal tree, can name it.
type ImportSpec = branchimport.Spec

// CreateChatInput is one CreateChat. ID is the id the caller already draws the
// row under, or "" to mint one; a taken id is refused by the chat aggregate.
type CreateChatInput struct {
	ID          string
	WorkspaceID string
	ProviderID  string
	ParentID    string
	Worktree    WorktreeSpec
	Surface     string
}

// CreatedChat is what a CreateChat made: the chat, the runner started on it,
// and the workspace a fork or import minted for it ("" for a plain chat).
type CreatedChat struct {
	ChatID      string
	RunnerID    string
	WorkspaceID string
}
