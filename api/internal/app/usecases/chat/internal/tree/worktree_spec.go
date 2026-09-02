package tree

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/branchimport"
)

// WorktreeMode says how a new chat comes by the worktree it owns, if it owns
// one at all.
//
// It replaces the ownWorktree bool CreateChat used to carry. A bool holds two
// states and the create has three: a plain bubble owns nothing, a fork mints a
// brand-new branch off its resolved fork parent, and an import adopts a branch
// that already exists. The third was previously reached only by a workspace-first
// path in another usecase that minted no chat at all, which is the whole bug
// (spec §0): a workspace with no owning chat is unreachable, because the only
// way to address a workspace is through the row that owns it.
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

// WorktreeSpec is CreateChat's worktree half: which of the three ways the new
// chat relates to a worktree, plus the details the import case needs.
//
// Import is read only when Mode is WorktreeImport and is ignored otherwise, so
// the zero value is exactly "a plain chat" — the same default the removed bool
// gave.
type WorktreeSpec struct {
	Mode   WorktreeMode
	Import ImportSpec
}

// ImportSpec describes the existing branch a WorktreeImport create adopts.
//
// It is an alias rather than a declaration of its own: the shared test doubles
// in usecases/mocks stand in for the Agent port this type appears on, and they
// cannot import anything under this feature's internal tree. See the
// branchimport package's own doc.
type ImportSpec = branchimport.Spec
