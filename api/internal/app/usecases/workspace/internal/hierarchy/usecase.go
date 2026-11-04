package hierarchy

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

// Usecase orchestrates the worktree hierarchy: worktree-backed create, local child→parent merge,
// re-parenting and cascade delete. All hierarchy guards live here.
type Usecase interface {
	CreateChild(
		ctx context.Context,
		in CreateChildInput,
	) (domain.Workspace, error)
	// CreateFromImport batch-imports branches as managed workspaces, PR-parented
	// up to a protected/default root, creating missing ancestors (07 §import).
	CreateFromImport(ctx context.Context, in ImportInput, p ImportProgress) error
	MergeIntoParent(
		ctx context.Context,
		childID string,
		strategy gitdomain.MergeStrategy,
	) (MergeResult, error)
	Reparent(
		ctx context.Context,
		childID string,
		newParentID string,
	) (domain.Workspace, error)
	// RenameBranch renames a managed workspace's branch and relocates its
	// workspace root to the directory the new name derives, so git, the
	// filesystem and the record stay in agreement.
	RenameBranch(
		ctx context.Context,
		wsID string,
		newBranch string,
	) (domain.Workspace, error)
	RebaseOntoParent(
		ctx context.Context,
		childID string,
	) (domain.Workspace, error)
	// RetryProvision re-runs holder resolution and provisioning for an existing placeholder, clearing
	// HeldByPath on success. Returns ErrBranchStillHeld when the branch is still live-held.
	RetryProvision(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	// DetachHolder frees a live holder off the placeholder's branch (with the
	// caller's consent), clears the home row's branch when the holder is the repo
	// home, then Retry-provisions in place (spec §3.5/§3.7).
	DetachHolder(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	// DeleteCascade removes rootID and its unlocked descendants. Without
	// consent it refuses, touching nothing, when that would destroy work that
	// exists nowhere else (domain.WorkAtRisked).
	DeleteCascade(
		ctx context.Context,
		rootID string,
		consent domain.DeleteConsent,
	) error
	// DeletionPlan is what DeleteCascade(rootID) would remove, or its refusal.
	DeletionPlan(ctx context.Context, rootID string) ([]string, error)
	// WorkAtRisk is what DeleteCascade of each root would refuse over.
	WorkAtRisk(
		ctx context.Context,
		rootIDs []string,
	) ([]domain.WorkAtRisk, error)
	// DeleteRepoWorkspaces removes every workspace of a repo (locked ones included, never by force) using
	// the repo the CALLER supplies: the cascade runs after the repo row is gone so its tombstone is not
	// held behind per-worktree git work, and without the row removeOne could not unregister worktrees.
	DeleteRepoWorkspaces(
		ctx context.Context,
		repo domain.Repository,
		consent domain.DeleteConsent,
	) error
	// RepoWorkAtRisk is what DeleteRepoWorkspaces would refuse over.
	RepoWorkAtRisk(
		ctx context.Context,
		repo domain.Repository,
	) ([]domain.WorkAtRisk, error)
	// SetChatObserver wires the working-chat check guardReparent needs. A setter, not a New option,
	// because the chat usecase depends on this one and does not exist yet at construction; nil disables
	// the guard.
	SetChatObserver(observer ChatWorkObserver)
	// SetOwningChats wires the chat-side surface every created workspace is born under; a setter for the
	// same construction-order reason as SetChatObserver.
	SetOwningChats(chats OwningChats)
}

// TerminalReaper terminates a workspace's live PTY sessions before its worktree is removed, so shells,
// fds and ring buffers do not leak. It lists by CHAT because the engine keys sessions per owning chat,
// and one worktree can carry several chats.
type TerminalReaper interface {
	ListSessionsForChat(chatID string) []string
	Kill(ctx context.Context, sessionID string) error
}

// ChatWorkObserver answers the two questions guardReparent's working-chat check
// needs: which chats a workspace owns, and whether one of them is working —
// the chat usecase's own answer (usecases/chat.Usecase.ChatBusy).
type ChatWorkObserver interface {
	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)
	ChatBusy(ctx context.Context, chatID string) (bool, error)
}

type hierarchyUsecase struct {
	workspaces   workspace.Workspace
	git          enginegit.Engine
	provider     engineprovider.Engine
	repos        store.Store[domain.Repository, string]
	now          func() time.Time
	crowbarHome  func() (string, error)
	terminals    TerminalReaper
	chatObserver ChatWorkObserver
	owningChats  OwningChats
	// nodes mints a workspace's own Node position row at creation. Never nil
	// (New defaults it to noOpNodeCreator); WithNodes overrides it with a real
	// one. A mint failure rolls the whole create back — see mintWorkspaceNode.
	nodes NodeCreator
}

// Option configures optional hierarchyUsecase dependencies without widening the
// New signature (it has many test call sites).
type Option func(*hierarchyUsecase)

// WithTerminalReaper wires the terminal engine so cascade delete can kill a
// workspace's PTY sessions. Without it, terminal cleanup is skipped (tests).
func WithTerminalReaper(r TerminalReaper) Option {
	return func(u *hierarchyUsecase) { u.terminals = r }
}

// New builds the hierarchy usecase. repos resolves a workspace's repository main
// path (via RepoID) so worktree removal and branch deletion run against the repo,
// never against a child's own worktree (07 §5).
func New(
	workspaces workspace.Workspace,
	git enginegit.Engine,
	provider engineprovider.Engine,
	repos store.Store[domain.Repository, string],
	now func() time.Time,
	crowbarHome func() (string, error),
	opts ...Option,
) Usecase {
	u := &hierarchyUsecase{
		workspaces:  workspaces,
		git:         git,
		provider:    provider,
		repos:       repos,
		now:         now,
		crowbarHome: crowbarHome,
		// Defaulted so u.nodes is NEVER nil; WithNodes overrides it. See
		// noOpNodeCreator's own doc (node.go).
		nodes: noOpNodeCreator{},
	}
	for _, o := range opts {
		o(u)
	}
	return u
}

// SetChatObserver implements Usecase.
func (u *hierarchyUsecase) SetChatObserver(observer ChatWorkObserver) {
	u.chatObserver = observer
}

// SetOwningChats implements Usecase.
func (u *hierarchyUsecase) SetOwningChats(chats OwningChats) {
	u.owningChats = chats
}
