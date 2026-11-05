package tree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

// The tree's ports and the shapes its writes take.

// Chats is the chat-aggregate surface this usecase needs. Get and ListByWorkspace serve the async
// projection, right for rendering; LoadChat folds from the event log and is always current, so placement
// decisions are taken on it.
type Chats interface {
	ListByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)
	// ListChats returns every chat the daemon knows, across every workspace and repo: a sidebar level spans
	// workspaces, so placement plans against all of them and lets the scope decide who counts.
	ListChats(
		ctx context.Context,
	) ([]domain.Chat, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	LoadChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	SetPlacement(
		ctx context.Context,
		chatID string,
		parentID string,
		order int,
	) (domain.Chat, error)
	// SetOrder answers problem.ErrNotFound for a chat a concurrent delete already purged; writeRow treats
	// it as nothing to do, since a vanished row needs no order.
	SetOrder(
		ctx context.Context,
		chatID string,
		order int,
	) (domain.Chat, error)
}

// Agent is the agent usecase as this one sees it: everything a chat is besides a row (its conversation,
// ledger and CLI). This usecase moves rows and decides which chats are created or erased; the agent does it.
type Agent interface {
	// MintChat creates the chat aggregate and starts nothing, so the row is placed before any CLI exists
	// and a thread's first session is told its lineage. providerID ("" for a row with no CLI) is recorded at mint;
	// surface is the view the chat is born on, "" for the provider's default.
	MintChat(
		ctx context.Context,
		chatID string,
		workspaceID string,
		providerID string,
		surface string,
	) (string, error)
	// OpenChat is the second half of the create MintChat opens, run once the row is
	// where it belongs: it refuses a provider no launch could start, and launches the
	// CLI only for a chat landing on its terminal ("" otherwise: its first prompt does).
	OpenChat(
		ctx context.Context,
		chatID string,
		providerID string,
	) (string, error)
	// SpawnChatWithOwnWorktree fills an already minted and placed chat's empty workspace slot with a worktree
	// forked from its resolved fork parent, then opens it as OpenChat does; ErrNoForkParent when it resolves
	// none. A blank branch gets a server-generated name.
	SpawnChatWithOwnWorktree(
		ctx context.Context,
		chatID string,
		providerID string,
		branch string,
	) (workspaceID, runnerID string, err error)
	// SpawnChatWithImportedWorktree fills a minted, placed chat's slot with an existing branch (discarded if
	// anything after fails), returning the workspace so the caller can judge the row's kind. An empty
	// providerID opens nothing: a batch import must not launch a CLI per branch.
	SpawnChatWithImportedWorktree(
		ctx context.Context,
		chatID string,
		providerID string,
		spec ImportSpec,
	) (ws domain.Workspace, runnerID string, err error)
	// AttachWorkspace points an already-minted chat at the workspace it owns, for creation paths that build
	// their own workspace.
	AttachWorkspace(
		ctx context.Context,
		chatID string,
		workspaceID string,
	) error
	// PurgeChat erases one chat outright: the aggregate, its CLI, its conversation history and its ledger.
	PurgeChat(
		ctx context.Context,
		chatID string,
	) error
	// NoteThreadLineage records in a chat's conversation that a move made it a thread of ancestors:
	// re-parenting takes effect from the move on, so the change is marked at that point.
	NoteThreadLineage(
		ctx context.Context,
		chatID string,
		ancestors []string,
	) error
}

// WorkspaceGitStatus is defined in home_ports.go.

// WorkspaceReaper tears down the worktree a deleted chat owned, since a workspace is reachable only through
// its owning chat. It is the same adapter a failed promotion's rollback uses, so the workspace door's
// guards apply here too; DeletionPlan and WorkAtRisk preview what a delete removes or would refuse over.
type WorkspaceReaper interface {
	DeletionPlan(ctx context.Context, workspaceID string) ([]string, error)
	DeleteWorkspace(
		ctx context.Context,
		workspaceID string,
		consent domain.DeleteConsent,
	) error
	WorkAtRisk(
		ctx context.Context,
		workspaceIDs []string,
	) ([]domain.WorkAtRisk, error)
}

// CreateInput creates a folder appended to its parent's sibling space: ParentID is a chat, folder or ""
// (the root). It carries a RepoID, not a workspace id, because a folder owns no workspace.
type CreateInput struct {
	ID       string
	RepoID   string
	ParentID string
	Name     string
	// HomeID is the home workspace a project-home folder (RepoID == "")
	// belongs to — see dtree.Folder.HomeID.
	HomeID string
}

// MoveInput is a partial folder placement change: a nil field is left as it is,
// so a reorder within one parent and a move to another are the same call. A move
// with no Order appends to the end of the destination.
type MoveInput struct {
	ParentID *string
	Order    *int
}

// PlaceInput is a partial chat placement change, mirroring MoveInput. A chat parent makes the chat a
// thread of it, a folder parent is organisation only; this usecase moves rows and need not tell them apart.
type PlaceInput struct {
	ParentID *string
	Order    *int
}

// ChatDeletion reports what a chat delete removed and moved. Chats and Folders are the erased ids, deepest
// first, kept apart because callers handle conversations and organisation differently.
type ChatDeletion struct {
	Chats   []string
	Folders []string
	Shifted []domain.Chat
	// Taken is the chats outside the selection the git cascade took with it.
	Taken []string
}

// Usecase owns the sidebar forest's folder CRUD, chat placement, and the dense
// sibling order every row kind shares. Every mutation leaves the affected
// levels renumbered 0..n-1.
type Usecase interface {
	// ListInRepo returns a repo's folder rows ("" for project home).
	ListInRepo(
		ctx context.Context,
		repoID string,
	) ([]domain.Chat, error)
	// ListInHome returns ONE project's home folder rows: those recorded
	// against homeID, plus legacy home folders that recorded no home.
	ListInHome(
		ctx context.Context,
		homeID string,
	) ([]domain.Chat, error)
	// FolderScope answers a folder's stored identity (its repo or home
	// scope), or problem.ErrNotFound — the fact a mount checks before it
	// lets a caller rename, move or delete the row.
	FolderScope(
		ctx context.Context,
		id string,
	) (dtree.Folder, error)
	// Create appends a new folder to its parent's level and densifies it, returning the folder plus every
	// other row shifted, so the caller broadcasts the whole change.
	Create(
		ctx context.Context,
		in CreateInput,
	) (domain.Chat, []domain.Chat, error)
	// Rename sets a folder's display name, leaving its placement untouched.
	Rename(
		ctx context.Context,
		id string,
		name string,
	) (domain.Chat, error)
	// Move re-parents and/or reorders a folder with its subtree, densifying both levels, and returns it plus
	// the rows shifted. ErrSubtreeWorking if any row in the subtree is working.
	Move(
		ctx context.Context,
		id string,
		in MoveInput,
	) (domain.Chat, []domain.Chat, error)
	// Delete removes a folder and promotes what it held to its parent, never cascading: a folder holds no
	// conversation, so the chats in it are only unfiled. It returns every row written.
	Delete(
		ctx context.Context,
		id string,
	) ([]domain.Chat, error)
	// CreateChat mints a chat, places it (a Node row at its level's next slot) and only then starts its CLI,
	// so a thread's first session knows its lineage. A bad parent is refused before minting and a later
	// failure removes the chat; WorktreeFork/Import also fill its workspace in one call (see WorktreeSpec).
	CreateChat(
		ctx context.Context,
		in CreateChatInput,
	) (CreatedChat, error)
	// ImportBranchAsChat materialises an existing branch chat-first with no CLI, placed by its git lineage,
	// returning the chat and workspace ids. A failure after the mint still returns the placed chat.
	ImportBranchAsChat(
		ctx context.Context,
		chatID string,
		spec ImportSpec,
	) (mintedChatID, workspaceID string, err error)
	// MintOwningChat mints and places the chat about to own a workspace, under parentWorkspaceID's own
	// Node row, for a creation path that builds the workspace itself (see owning_chat.go).
	MintOwningChat(
		ctx context.Context,
		chatID string,
		parentWorkspaceID string,
	) (string, error)
	// AttachOwningWorkspace points a chat minted by MintOwningChat at the workspace it was minted for.
	AttachOwningWorkspace(
		ctx context.Context,
		chatID string,
		ws domain.Workspace,
	) error
	// DiscardOwningChat takes a chat minted by MintOwningChat back out, for a
	// caller whose own workspace creation then failed.
	DiscardOwningChat(
		ctx context.Context,
		chatID string,
	) error
	// PlaceChat moves a chat with its subtree within workspaceID's tree, returning it plus the rows shifted;
	// a chat outside workspaceID is not found, a working subtree is ErrSubtreeWorking. Gaining a chat
	// ancestor is noted in the conversation, since it applies from the move on (see noteNewAncestors).
	PlaceChat(
		ctx context.Context,
		workspaceID string,
		chatID string,
		in PlaceInput,
	) (domain.Chat, []domain.Chat, error)
	// DeleteChat erases a chat and its whole subtree, deepest first: a thread continues its parent and cannot
	// outlive it. Refused, with no override, while any row in it is working; work at risk in a reaped
	// worktree refuses it too unless consent discards it.
	DeleteChat(
		ctx context.Context,
		chatID string,
		consent domain.DeleteConsent,
	) (ChatDeletion, error)
	// PlaceWorkspace moves a workspace's own row (locked branch or ordinary fork) within its repo's tree;
	// problem.ErrNotFound for a workspace with no repo, the project home included.
	PlaceWorkspace(
		ctx context.Context,
		workspaceID string,
		in PlaceInput,
	) (domain.Chat, []domain.Chat, error)
}
