package tree

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/branchimport"
)

type WorktreeMode int

const (
	WorktreeNone WorktreeMode = iota
	WorktreeFork
	WorktreeImport
)

type WorktreeSpec struct {
	Mode   WorktreeMode
	Branch string
	Import ImportSpec
}

type ImportSpec = branchimport.Spec

type CreateChatInput struct {
	ID          string
	WorkspaceID string
	ProviderID  string
	ParentID    string
	Worktree    WorktreeSpec
	Surface     string
}

type CreatedChat struct {
	ChatID      string
	RunnerID    string
	WorkspaceID string
}
