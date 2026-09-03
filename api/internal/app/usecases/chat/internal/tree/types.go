package tree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The tree's ports and the shapes its writes take.

type Store interface {
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.ChatFolder, error)
	FindWhere(
		ctx context.Context,
		match domain.ChatFolder,
	) ([]domain.ChatFolder, error)
	Save(
		ctx context.Context,
		folder domain.ChatFolder,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
}

// Chats is the chat-aggregate surface this usecase needs. It reads the
// workspace's chats because chats and folders share one sibling space, and it
// writes placement — which for a chat is lineage, not decoration.
//
// Both the reads and the writes split along one line: what an operation DECIDES
// against what it merely carries. LoadChat folds the SUBJECT from the event log,
// because its stored parent is what a move is planned against; ListByWorkspace
// serves the projection, which is right for the rest of the level since that is
// read to renumber and never to decide a parent from. SetPlacement writes the
// row the caller moved, SetOrder every other row a densify touched.
type Chats interface {
	ListByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)
	GetChat(
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
	SetOrder(
		ctx context.Context,
		chatID string,
		order int,
	) (domain.Chat, error)
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
	// PurgeChat erases one chat outright — the aggregate, the CLI pointed at it,
	// its conversation history and its on-disk ledger. This usecase decides WHICH
	// chats a delete takes and knows nothing about how one is torn down.
	PurgeChat(
		ctx context.Context,
		chatID string,
	) error
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

// CreateInput carries the fields needed to create a chat folder. ParentID is a
// chat id, another folder's id, or "" for the panel root; the new folder is
// appended at the end of that sibling space.
type CreateInput struct {
	ID          string
	WorkspaceID string
	ParentID    string
	Name        string
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
// subtree is a plain row with no projection to announce it, so the caller
// broadcasts those itself.
type ChatDeletion struct {
	Chats   []string
	Folders []string
	Shifted []domain.ChatFolder
}

// Usecase owns Chats-panel folder CRUD, chat placement, and the dense sibling
// order the two kinds share. Every mutation leaves the affected levels
// renumbered 0..n-1.
type Usecase interface {
	// ListInWorkspace returns one workspace's chat folders.
	ListInWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.ChatFolder, error)
	// Create appends a new folder to the end of its parent's sibling space and
	// densifies that level. It returns the new folder plus every OTHER folder the
	// densify shifted, so the caller broadcasts the whole change rather than one
	// row of it — the shifted rows' orders are otherwise stale in every client
	// cache until the next reconnect. Shifted CHAT rows need no such handling:
	// their write is an aggregate command, and the hub projection broadcasts every
	// one.
	Create(
		ctx context.Context,
		in CreateInput,
	) (domain.ChatFolder, []domain.ChatFolder, error)
	// Rename sets a folder's display name, leaving its placement untouched.
	Rename(
		ctx context.Context,
		workspaceID string,
		id string,
		name string,
	) (domain.ChatFolder, error)
	// Move re-parents and/or reorders a folder, densifying both the level it left
	// and the level it joined. It returns the moved folder plus every other folder
	// those two densifies shifted.
	Move(
		ctx context.Context,
		workspaceID string,
		id string,
		in MoveInput,
	) (domain.ChatFolder, []domain.ChatFolder, error)
	// Delete removes a folder and PROMOTES what it held to the folder's own
	// parent. It never cascades: a folder holds no conversation, so deleting the
	// chats filed under it would destroy work the user only meant to unfile. It
	// returns every folder row the promotion and densify wrote.
	Delete(
		ctx context.Context,
		workspaceID string,
		id string,
	) ([]domain.ChatFolder, error)
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
	// A parentID naming nothing, or a row in another workspace, is refused BEFORE
	// anything is minted or spawned, with the errors placement already returns. A
	// failure after the mint takes the chat back out: a create the user was told
	// failed must not leave a chat behind.
	CreateChat(
		ctx context.Context,
		workspaceID string,
		providerID string,
		parentID string,
	) (chatID, runnerID string, err error)
	// PlaceChat moves a chat within the tree and reorders it in its new level. It
	// returns the placed chat plus every folder the densify shifted.
	//
	// A move that gives the chat a CHAT ancestor it did not have is also recorded
	// in that chat's own conversation, because it takes effect from the move
	// onward and never retroactively: fifty turns already had without that
	// context stay fifty turns had without it. See noteNewAncestors.
	//
	// workspaceID is the scope the caller is acting in, and the move is refused if
	// the chat is not in it: a chat addressed from the wrong workspace is not this
	// caller's row to move.
	PlaceChat(
		ctx context.Context,
		workspaceID string,
		chatID string,
		in PlaceInput,
	) (domain.Chat, []domain.ChatFolder, error)
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
	DeleteChat(
		ctx context.Context,
		chatID string,
	) (ChatDeletion, error)
}
