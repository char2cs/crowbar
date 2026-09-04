package tree

import (
	"context"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The tree's ports and the shapes its writes take.

// Chats is the chat-aggregate surface this usecase needs. Folder rows and
// conversation rows are the SAME aggregate now (Chat.Type distinguishes them),
// so one port serves both: what used to be a separate ChatFolder table's
// Create/FindByKey/FindWhere/Save/Delete is now these same Create/Get/List/
// SetTitle/SetPlacement/SetOrder/Forget calls, exactly as a conversation row
// already used them.
//
// Get and LoadChat answer different questions. Get serves the read-model
// projection, right for rendering a list. LoadChat folds the chat directly
// from the event log, so it is always current — the read a placement decision
// must be taken on, never the projection: a chat read back straight after a
// placement can still be serving the placement it had BEFORE.
type Chats interface {
	ListByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)
	// ListChats returns every row the daemon knows, across every workspace and
	// repo. Folder CRUD plans against it because a folder carries no workspace
	// of its own to scope a narrower read by — the repo boundary a real
	// ListInRepo would enforce is a walk over ParentID that does not exist yet
	// (stage 3); this task only retypes the storage.
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
	// Create mints a bare, unplaced row — a folder at the panel root — the same
	// two-phase shape MintChat already uses: minted first, placed second, so a
	// row that fails to place never sits half-created in a container it was
	// never checked against.
	Create(
		ctx context.Context,
		in agentchat.CreateInput,
	) (domain.Chat, error)
	SetTitle(
		ctx context.Context,
		chatID string,
		title string,
		source string,
	) (domain.Chat, error)
	SetPlacement(
		ctx context.Context,
		chatID string,
		parentID string,
		order int,
	) (domain.Chat, error)
	SetOrder(
		ctx context.Context,
		chatID string,
		order int,
	) (domain.Chat, error)
	// SetType rewrites which KIND of row a chat is and touches nothing else —
	// same id, same placement, same conversation. The backfill needs it because
	// what a workspace IS can change after its owning row is minted: an ordinary
	// worktree that is later locked owns a BRANCH row from then on, and retyping
	// the row it already has is what keeps that workspace from acquiring a
	// second one.
	SetType(
		ctx context.Context,
		chatID string,
		chatType domain.ChatType,
	) (domain.Chat, error)
	// Forget erases a row outright. A folder holds no runner and no ledger, so
	// this is the whole of what deleting one means — unlike Agent.PurgeChat,
	// which also tears down the CLI and the conversation a CHAT row carries.
	Forget(
		ctx context.Context,
		id string,
	) error
}

// Agent is the agent usecase as this one sees it: the collaborator that owns the
// AgentChat aggregate itself and the vendor CLIs pointed at it.
//
// It is ONE port rather than one per verb because it is one collaborator from
// here: this usecase moves ROWS, and everything a chat is besides a row — its
// conversation, its ledger, the process talking to it — lives behind this port,
// with nothing in this package learning how any of it works. Which agent concern
// answers which method is the CONTAINER's business (see usecases.agentChatTree),
// not this one's.
type Agent interface {
	// SpawnChat mints a chat at the panel root and starts a CLI on it in one call.
	// It is the UNPLACED create kept whole rather than reassembled from MintChat
	// and StartRunner: a chat going nowhere has no edge to write, so there is no
	// gap to open between the two halves, and splitting it would change the order a
	// plain new chat is created in for no reason.
	SpawnChat(
		ctx context.Context,
		workspaceID string,
		providerID string,
	) (chatID, runnerID string, err error)
	// MintChat creates the chat aggregate and starts nothing, so this usecase can
	// place the new row BEFORE any CLI exists. A chat under another chat is a
	// thread of it and is told its lineage at spawn, so the parent edge has to be
	// written first — a create that spawned first would leave the thread's first
	// session, the one the user is watching, believing it is a standalone chat.
	MintChat(
		ctx context.Context,
		workspaceID string,
	) (string, error)
	// StartRunner launches a vendor CLI on a chat that already exists — the second
	// half of the create MintChat opens, run once the row is where it belongs.
	StartRunner(
		ctx context.Context,
		chatID string,
		providerID string,
	) (string, error)
	// SpawnChatWithOwnWorktree is StartRunner's ownWorktree counterpart: chatID
	// has already been minted and placed (so its lineage — and here, the fork
	// parent its own walk resolves — is fixed before its first CLI exists,
	// exactly like the plain thread path above), but has never had a runner. This
	// fills its empty workspace slot with a fresh worktree forked from its
	// resolved fork parent, then starts providerID's CLI in it — composing the
	// SAME worktree-provisioning port Promote uses (model spec §4.2) for a chat
	// with no existing runner to tear down and respawn.
	//
	// It refuses with ErrNoForkParent (see promote.go) when chatID's own walk
	// resolves no ancestor carrying a workspace — there is nothing to fork from.
	SpawnChatWithOwnWorktree(
		ctx context.Context,
		chatID string,
		providerID string,
	) (runnerID string, err error)
	// SpawnChatWithImportedWorktree is SpawnChatWithOwnWorktree's IMPORT
	// counterpart, with the identical contract: chatID has already been minted
	// and placed, and this fills its empty workspace slot — with a branch that
	// ALREADY EXISTS, described by spec, rather than a fresh fork — then starts
	// providerID's CLI in it, discarding the workspace again if anything after
	// it fails.
	//
	// It returns the workspace as well as the runner because the CALLER decides
	// what kind of row the chat is (see createImportedWorktreeChat), and a
	// locked branch — which is what most imports produce — owns a branch row
	// rather than an ordinary chat row. That judgement is taken from the
	// workspace itself, so the workspace has to come back.
	//
	// An empty providerID starts NO runner and returns an empty runner id. That
	// is the repo-add and batch-import case: materialising twenty branches must
	// not launch twenty vendor CLIs, and a branch row is a row in the sidebar,
	// not a conversation somebody is having.
	SpawnChatWithImportedWorktree(
		ctx context.Context,
		chatID string,
		providerID string,
		spec ImportSpec,
	) (ws domain.Workspace, runnerID string, err error)
	// AttachWorkspace points an already-minted chat at the workspace it owns.
	// It is the bare write behind MintOwningChat's second step (owning_chat.go),
	// for the creation paths that build their own workspace and only need the
	// row pointed at it.
	AttachWorkspace(
		ctx context.Context,
		chatID string,
		workspaceID string,
	) error
	// PurgeChat erases one chat outright — the aggregate, the CLI pointed at it,
	// its conversation history and its on-disk ledger. This usecase decides WHICH
	// chats a delete takes and knows nothing about how one is torn down.
	PurgeChat(
		ctx context.Context,
		chatID string,
	) error
	// HasTurns reports whether anything was ever SAID in a chat. The boot
	// backfill asks it before adopting a row into a different kind, and it is
	// the only question that separates the two rows this package cannot
	// otherwise tell apart: an owning row the backfill itself minted for a
	// worktree, and an ordinary conversation that merely started inside the same
	// workspace. Both are chat-typed and both carry the workspace id; only one
	// of them has anybody's words in it.
	HasTurns(
		ctx context.Context,
		chatID string,
	) (bool, error)
	// NoteThreadLineage records, in a chat's own conversation, that a move has just
	// made it a thread of the chats named. It exists because re-parenting takes
	// effect FROM THE MOVE ONWARD: a chat that gains an ancestor does not
	// retroactively acquire the context it did not have while it was talking, and
	// the only way to say where the change happened is to say it at that point in
	// the conversation.
	NoteThreadLineage(
		ctx context.Context,
		chatID string,
		ancestors []string,
	) error
}

// WorkspaceGitStatus is the narrow read port DeletePreview needs off the
// workspace usecase: each workspace's own already-synced Added/Deleted
// working-tree counts (00 §5.3) — the same numbers the sidebar itself
// renders, never a live git call. A preview runs before every idle delete
// confirm, so it has to stay as cheap as the read model it draws from.
type WorkspaceGitStatus interface {
	WorkingTreeSummary(
		ctx context.Context,
		workspaceID string,
	) (added, deleted int, err error)
}

// WorkspaceRoster is the boot backfill's census: every workspace the daemon
// knows, across every repo, tombstones included (they are filtered here — see
// liveWorkspaces). It is a second port rather than a method on
// WorkspaceGitStatus because the two are asked at opposite moments for
// opposite reasons: one answers a per-row question on a hot user path, this
// one is read exactly once, at startup.
type WorkspaceRoster interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
}

// WorkspaceReaper is the narrow write port DeleteChat needs: tearing down the
// worktree a chat OWNED, in the same breath the chat is erased.
//
// Without it a delete violated the invariant this whole surface exists to hold.
// A workspace is reachable only through the chat that owns it, so deleting that
// chat and leaving the workspace behind produced exactly the orphan spec §0
// diagnosed — a real worktree on disk with nothing anywhere able to name it —
// from the opposite direction to the import path that first produced one.
//
// The method name is the one the port it is satisfied by already uses
// (chat.WorktreeCreator.DiscardChildWorkspace), and deliberately so: the
// container hands this the SAME adapter, so a delete here and a failed
// promotion's rollback tear a workspace down through one call
// (hierarchy.DeleteCascade) rather than two ways that could diverge. Its own
// guards — a locked root, a subtree owning a working chat — therefore apply
// here too, which is correct: a protected branch's worktree is no more
// deletable through the chat door than through the workspace one.
type WorkspaceReaper interface {
	DiscardChildWorkspace(
		ctx context.Context,
		workspaceID string,
	) error
}

// CreateInput carries the fields needed to create a folder. ParentID is a
// chat id, another folder's id, or "" for the panel root; the new folder is
// appended at the end of that sibling space. RepoID is carried rather than a
// workspace id because a folder owns none — see the package doc.
type CreateInput struct {
	ID       string
	RepoID   string
	ParentID string
	Name     string
}

// MoveInput is a partial folder placement change: a nil field is left as it is,
// so a reorder within one parent and a move to another are the same call. A move
// with no Order appends to the end of the destination.
type MoveInput struct {
	ParentID *string
	Order    *int
}

// PlaceInput is a partial chat placement change, mirroring MoveInput. A
// ParentID naming another CHAT makes this chat a THREAD of it — it will read
// that chat's turns, and its chat ancestors' in turn — while a ParentID naming a
// FOLDER is organisation only. This usecase does not distinguish them: it moves
// rows, and the meaning follows from what the id turns out to name.
type PlaceInput struct {
	ParentID *string
	Order    *int
}

// ChatDeletion reports what a chat delete actually removed and what it moved.
//
// Chats and Folders are the ids that no longer exist, deepest first. They are
// returned rather than merely logged because each one has to reach every client:
// a purged chat rides its own aggregate frame, but a folder caught inside the
// subtree is now the same aggregate kind, and its erasure rides its own frame
// too — this split is kept because the CALLER still needs to know which ids were
// conversations (torn down through the agent usecase) versus organisation only.
type ChatDeletion struct {
	Chats   []string
	Folders []string
	Shifted []domain.Chat
}

// Usecase owns the sidebar forest's folder CRUD, chat placement, and the dense
// sibling order every row kind shares. Every mutation leaves the affected
// levels renumbered 0..n-1.
type Usecase interface {
	// ListInRepo returns a repo's folder rows. See Chats.ListChats: the repo
	// boundary itself is not yet enforced, only the row kind is filtered.
	ListInRepo(
		ctx context.Context,
		repoID string,
	) ([]domain.Chat, error)
	// Create appends a new folder to the end of its parent's sibling space and
	// densifies that level. It returns the new folder plus every OTHER row the
	// densify shifted, so the caller broadcasts the whole change rather than one
	// row of it — the shifted rows' orders are otherwise stale in every client
	// cache until the next reconnect.
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
	// Move re-parents and/or reorders a folder, densifying both the level it left
	// and the level it joined. It returns the moved folder plus every other row
	// those two densifies shifted.
	//
	// The move takes the folder's whole subtree with it — nothing below it is
	// rewritten, since a child's ParentID already names it — so it is refused
	// with ErrSubtreeWorking if any row in that subtree, folder or chat, is
	// currently working.
	Move(
		ctx context.Context,
		id string,
		in MoveInput,
	) (domain.Chat, []domain.Chat, error)
	// Delete removes a folder and PROMOTES what it held to the folder's own
	// parent. It never cascades: a folder holds no conversation, so deleting the
	// chats filed under it would destroy work the user only meant to unfile. It
	// returns every row the promotion and densify wrote.
	Delete(
		ctx context.Context,
		id string,
	) ([]domain.Chat, error)
	// CreateChat mints a chat, places it under parentID, and starts providerID's
	// vendor CLI on it — in that order, which is the whole contract.
	//
	// A chat under another CHAT is a thread of it and is handed its lineage at
	// spawn, so the edge has to be on the aggregate before the CLI exists. Spawning
	// first leaves the thread's FIRST session not knowing it is a thread — the one
	// session the user is most likely to be watching, having just asked for a
	// thread and typed a question into it. A parentID naming a FOLDER takes the
	// identical path ("new chat in this folder"); only the lineage it resolves
	// differs, which is the folder rule doing its job rather than a second case.
	//
	// An empty parentID is a plain new chat at the panel root, passed straight
	// through to the unplaced spawn and unchanged in every respect.
	//
	// A parentID naming nothing, or a chat in another workspace, is refused BEFORE
	// anything is minted or spawned, with the errors placement already returns. A
	// failure after the mint takes the chat back out: a create the user was told
	// failed must not leave a chat behind.
	//
	// worktree is model spec §4.1/§5.1's atomic create, in the three states the
	// create actually has (see WorktreeSpec). WorktreeFork and WorktreeImport
	// both mint and place the new chat exactly as the plain case above
	// (workspaceID is ignored for them — the row is a bubble until its worktree
	// exists), then fill its workspace slot in the SAME call: a fresh fork off
	// the resolved fork parent, or an existing branch adopted from the spec.
	// There is never a chat-less workspace, nor a workspace-less chat waiting to
	// be promoted, observable in between.
	//
	// Because such a chat has no workspace to conflict with yet, the
	// cross-workspace refusal above does not apply to it: a parentID naming a
	// chat that already owns a worktree of its own is an acceptable fork point,
	// the same way a BRANCH parent already is. See createOwnWorktreeChat
	// (chats.go), createImportedWorktreeChat (imported_chat.go), and the two
	// agent verbs behind them.
	CreateChat(
		ctx context.Context,
		workspaceID string,
		providerID string,
		parentID string,
		worktree WorktreeSpec,
	) (chatID, runnerID string, err error)
	// ImportBranchAsChat materialises one existing branch chat-first and hands
	// back both ids: the chat minted for it and the workspace it now owns. It
	// starts no vendor CLI — a batch import materialises rows, not conversations
	// — and resolves its own placement from the spec's git lineage.
	ImportBranchAsChat(
		ctx context.Context,
		spec ImportSpec,
	) (chatID, workspaceID string, err error)
	// MintOwningChat mints and places the chat that is about to own a workspace,
	// for a creation path that builds the workspace itself. parentWorkspaceID is
	// the git-lineage parent; the placement it resolves is that workspace's own
	// owning chat. See owning_chat.go for why this is three verbs rather than
	// one call.
	MintOwningChat(
		ctx context.Context,
		parentWorkspaceID string,
	) (chatID string, err error)
	// AttachOwningWorkspace points a chat minted by MintOwningChat at the
	// workspace it was minted for, retyping it as a branch row when the
	// workspace is one the sidebar draws as a branch.
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
	// PlaceChat moves a chat within the tree and reorders it in its new level. It
	// returns the placed chat plus every row the densify shifted.
	//
	// A move that gives the chat a CHAT ancestor it did not have is also recorded
	// in that chat's own conversation, because it takes effect from the move
	// onward and never retroactively: fifty turns already had without that
	// context stay fifty turns had without it. See noteNewAncestors.
	//
	// workspaceID is the scope the caller is acting in, and the move is refused if
	// the chat is not in it: a chat addressed from the wrong workspace is not this
	// caller's row to move.
	//
	// The move takes the chat's whole subtree with it, and is refused with
	// ErrSubtreeWorking if the chat or any row below it is currently working —
	// the same refusal Move makes for a folder.
	PlaceChat(
		ctx context.Context,
		workspaceID string,
		chatID string,
		in PlaceInput,
	) (domain.Chat, []domain.Chat, error)
	// DeleteChat erases a chat AND EVERY DESCENDANT — every threaded chat below
	// it, purged one aggregate at a time, and every folder caught inside that
	// subtree.
	//
	// This is the opposite of Delete's promotion, and deliberately so. A thread is
	// not filed under its parent, it CONTINUES it: it reads that chat's turns
	// whenever it asks. Promoting it would leave a conversation whose whole
	// premise — the context above it — has been deleted, and no drag can restore
	// what it used to read. The subtree goes deepest first so no intermediate
	// state ever has a chat pointing at a parent that is already gone.
	//
	// It is refused with ErrSubtreeWorking if the chat or any row below it is
	// currently working, checked BEFORE anything is purged. Unlike a locked
	// workspace, this refusal has no confirm-and-override path.
	DeleteChat(
		ctx context.Context,
		chatID string,
	) (ChatDeletion, error)
	// BackfillOwningChats gives every workspace the owning chat row it is owed,
	// once, at startup — minting one where there is none, and adopting the row a
	// workspace already has where that workspace has since become something
	// else. It is the migration for every workspace made before a workspace and
	// the chat that owns it were minted in one breath: the sidebar addresses a
	// workspace's placement BY that row, so a workspace without one exists on
	// disk and nowhere in the tree. See backfill.go.
	BackfillOwningChats(
		ctx context.Context,
	) error
	// EnsureOwningChat is BackfillOwningChats narrowed to ONE workspace, for the
	// moment a workspace changes character while the daemon is RUNNING rather
	// than between boots — a branch the user locks, or one a provider poll
	// reports protected, is branch-destined from that instant and everything
	// downstream expects its branch row to be there already. It takes the same
	// decision by the same code; see backfill.go.
	EnsureOwningChat(
		ctx context.Context,
		ws domain.Workspace,
	) error
	// DeletePreview answers what DeleteChat (a chat root) or Delete's cascading
	// successor (a folder root) is ABOUT to take, without taking it: every CHAT
	// row in the subtree, and the working-tree file count summed across every
	// workspace-owning row in it. A subtree can span more than one independent
	// workspace now, so this is the one place that count is actually computed
	// rather than read off a single workspace the caller already has.
	DeletePreview(
		ctx context.Context,
		chatID string,
	) (chatCount, fileCount int, err error)
}
