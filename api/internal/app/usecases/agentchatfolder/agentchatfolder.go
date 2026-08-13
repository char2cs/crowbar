// Package agentchatfolder owns the Chats panel's tree: the AgentChatFolder rows
// a user files chats into, where each chat hangs, and the dense sibling order
// the two kinds SHARE — they interleave at every level and sort on one Order
// field within one ParentID.
//
// Its two deletes are deliberately opposite, and the asymmetry is the whole
// domain rule. Deleting a FOLDER promotes what it held to the folder's own
// parent: a folder holds no conversation, so the chats outlive it. Deleting a
// CHAT takes its entire subtree: a thread exists to CONTINUE its parent — it
// reads that parent's turns — so leaving it behind would strand it reading a
// context that no longer exists.
//
// Nothing here reasons about processes. A chat's runner, its PTY and its ledger
// belong to the agent usecase; this one moves rows and, for the cascade, asks
// that usecase to erase each chat it has decided must go.
//
// Every operation reads the workspace's rows ONCE, plans the whole change in
// memory, and then writes only the rows that actually moved. That is not merely
// an optimisation: the chat read model is an asynchronous projection, so a
// re-read taken between a write and the renumber that follows it can still be
// serving the pre-write list. Planning from a single snapshot removes the race
// rather than papering over it with a barrier.
package agentchatfolder

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the chat-folder table surface this usecase needs: fetch one row by
// id, list a workspace's rows without materialising the whole table, persist,
// remove.
type Store interface {
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.AgentChatFolder, error)
	FindWhere(
		ctx context.Context,
		match domain.AgentChatFolder,
	) ([]domain.AgentChatFolder, error)
	Save(
		ctx context.Context,
		folder domain.AgentChatFolder,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
}

// Chats is the chat-aggregate surface this usecase needs. It reads the
// workspace's chats because chats and folders share one sibling space, and it
// writes placement — which for a chat is lineage, not decoration.
type Chats interface {
	ListByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.AgentChat, error)
	GetChat(
		ctx context.Context,
		id string,
	) (domain.AgentChat, error)
	SetPlacement(
		ctx context.Context,
		chatID string,
		parentID string,
		order int,
	) (domain.AgentChat, error)
}

// Agent is the agent usecase as this one sees it: the collaborator that owns the
// AgentChat aggregate itself and the vendor CLIs pointed at it.
//
// It is ONE port rather than one per verb because it is one collaborator, and
// because four separately-named ports that every caller satisfies with the same
// value are four chances to pass them in the wrong order at the only call site
// there is, for no isolation gained. This usecase moves ROWS; everything a chat is
// besides a row — its conversation, its ledger, the process talking to it — lives
// behind here, and nothing in this package learns how any of it works.
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
	Shifted []domain.AgentChatFolder
}

// Usecase owns Chats-panel folder CRUD, chat placement, and the dense sibling
// order the two kinds share. Every mutation leaves the affected levels
// renumbered 0..n-1.
type Usecase interface {
	// ListInWorkspace returns one workspace's chat folders.
	ListInWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.AgentChatFolder, error)
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
	) (domain.AgentChatFolder, []domain.AgentChatFolder, error)
	// Rename sets a folder's display name, leaving its placement untouched.
	Rename(
		ctx context.Context,
		workspaceID string,
		id string,
		name string,
	) (domain.AgentChatFolder, error)
	// Move re-parents and/or reorders a folder, densifying both the level it left
	// and the level it joined. It returns the moved folder plus every other folder
	// those two densifies shifted.
	Move(
		ctx context.Context,
		workspaceID string,
		id string,
		in MoveInput,
	) (domain.AgentChatFolder, []domain.AgentChatFolder, error)
	// Delete removes a folder and PROMOTES what it held to the folder's own
	// parent. It never cascades: a folder holds no conversation, so deleting the
	// chats filed under it would destroy work the user only meant to unfile. It
	// returns every folder row the promotion and densify wrote.
	Delete(
		ctx context.Context,
		workspaceID string,
		id string,
	) ([]domain.AgentChatFolder, error)
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
	) (domain.AgentChat, []domain.AgentChatFolder, error)
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

type chatFolderUsecase struct {
	folders Store
	chats   Chats
	agent   Agent
}

// New builds the chat-folder usecase over the folders table, the chat row
// repository, and the agent usecase behind everything a chat is besides a row.
//
// The chat handle is required because the two row kinds share one sibling space:
// renumbering a level without it would leave folders and chats holding
// independent, colliding indices. The agent handle is required in both
// directions — this usecase decides which chats a delete takes and which chat a
// create is born under, and the agent usecase is the only thing that knows how to
// erase one, mint one, or start a CLI on one.
func New(
	folders Store,
	chats Chats,
	agent Agent,
) Usecase {
	return &chatFolderUsecase{folders: folders, chats: chats, agent: agent}
}

func (u *chatFolderUsecase) ListInWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.AgentChatFolder, error) {
	rows, err := u.folders.FindWhere(ctx, domain.AgentChatFolder{WorkspaceID: workspaceID})
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: list in workspace: %w", err)
	}
	return rows, nil
}

func (u *chatFolderUsecase) Create(
	ctx context.Context,
	in CreateInput,
) (domain.AgentChatFolder, []domain.AgentChatFolder, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return domain.AgentChatFolder{}, nil, err
	}
	snapshot, err := u.snapshot(ctx, in.WorkspaceID)
	if err != nil {
		return domain.AgentChatFolder{}, nil, err
	}
	if cErr := u.checkContainer(ctx, snapshot, in.WorkspaceID, in.ParentID); cErr != nil {
		return domain.AgentChatFolder{}, nil, cErr
	}
	id := in.ID
	if id == "" {
		id = uuid.NewString()
	}
	target := snapshot.plan.NextSlot(in.ParentID)
	snapshot.add(domain.AgentChatFolder{
		ID:          id,
		WorkspaceID: in.WorkspaceID,
		ParentID:    in.ParentID,
		Name:        name,
		Order:       target,
	})
	snapshot.plan.Reorder(in.ParentID, id, target)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.AgentChatFolder{}, nil, err
	}
	return *snapshot.placedFolder(id), without(written, id), nil
}

func (u *chatFolderUsecase) Rename(
	ctx context.Context,
	workspaceID string,
	id string,
	name string,
) (domain.AgentChatFolder, error) {
	clean, err := cleanName(name)
	if err != nil {
		return domain.AgentChatFolder{}, err
	}
	current, err := u.load(ctx, workspaceID, id)
	if err != nil {
		return domain.AgentChatFolder{}, err
	}
	current.Name = clean
	if err := u.folders.Save(ctx, current); err != nil {
		return domain.AgentChatFolder{}, fmt.Errorf("agent chat folder: rename %s: save: %w", id, err)
	}
	return current, nil
}

func (u *chatFolderUsecase) Move(
	ctx context.Context,
	workspaceID string,
	id string,
	in MoveInput,
) (domain.AgentChatFolder, []domain.AgentChatFolder, error) {
	current, err := u.load(ctx, workspaceID, id)
	if err != nil {
		return domain.AgentChatFolder{}, nil, err
	}
	snapshot, err := u.snapshot(ctx, current.WorkspaceID)
	if err != nil {
		return domain.AgentChatFolder{}, nil, err
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	if mErr := u.checkMove(ctx, snapshot, current.WorkspaceID, id, destination); mErr != nil {
		return domain.AgentChatFolder{}, nil, mErr
	}
	u.replace(snapshot, id, current.ParentID, destination, in.Order)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.AgentChatFolder{}, nil, err
	}
	return *snapshot.placedFolder(id), without(written, id), nil
}

func (u *chatFolderUsecase) Delete(
	ctx context.Context,
	workspaceID string,
	id string,
) ([]domain.AgentChatFolder, error) {
	current, err := u.load(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	snapshot, err := u.snapshot(ctx, current.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if err := u.folders.Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("agent chat folder: delete %s: %w", id, err)
	}
	snapshot.plan.Reparent(id, current.ParentID)
	snapshot.drop(id)
	snapshot.plan.Reorder(current.ParentID, "", -1)
	return u.persist(ctx, snapshot)
}

func (u *chatFolderUsecase) CreateChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
	parentID string,
) (string, string, error) {
	if parentID == "" {
		return u.agent.SpawnChat(ctx, workspaceID, providerID)
	}
	if err := u.checkNewChatParent(ctx, workspaceID, parentID); err != nil {
		return "", "", err
	}
	chatID, err := u.agent.MintChat(ctx, workspaceID)
	if err != nil {
		return "", "", fmt.Errorf("agent chat folder: create chat: %w", err)
	}
	if _, _, pErr := u.PlaceChat(ctx, workspaceID, chatID, PlaceInput{ParentID: &parentID}); pErr != nil {
		return "", "", u.discard(ctx, chatID, pErr)
	}
	runnerID, err := u.agent.StartRunner(ctx, chatID, providerID)
	if err != nil {
		return "", "", u.discard(ctx, chatID, err)
	}
	return chatID, runnerID, nil
}

// checkNewChatParent refuses a destination a new chat cannot be born into, BEFORE
// anything is minted: a row this workspace does not hold, or one belonging to
// another. It is the same guard a move makes, minus the cycle test — a chat that
// does not exist yet cannot be inside its own subtree.
//
// It runs first so the ordinary failure costs nothing. Leaving it to PlaceChat
// would mint a chat, broadcast it, refuse the placement and then purge it again,
// which is a create and a delete on every mistyped id.
func (u *chatFolderUsecase) checkNewChatParent(
	ctx context.Context,
	workspaceID string,
	parentID string,
) error {
	snapshot, err := u.snapshot(ctx, workspaceID)
	if err != nil {
		return err
	}
	return u.checkContainer(ctx, snapshot, workspaceID, parentID)
}

// discard takes a just-minted chat back out when the create failed after minting
// it, and hands back the failure that caused it.
//
// The purge is best-effort and NEVER replaces the cause. The user asked to create
// a chat and that is what failed, so that is what they are told; a purge that
// fails on top of it leaves a placed, CLI-less chat, which is visible in the panel
// and deletable by hand — a far better outcome than reporting an error other than
// the one that actually happened.
func (u *chatFolderUsecase) discard(
	ctx context.Context,
	chatID string,
	cause error,
) error {
	if err := u.agent.PurgeChat(ctx, chatID); err != nil {
		slog.WarnContext(ctx, "agent chat folder: discard half-created chat",
			"err", err, "chat_id", chatID)
	}
	return cause
}

func (u *chatFolderUsecase) PlaceChat(
	ctx context.Context,
	workspaceID string,
	chatID string,
	in PlaceInput,
) (domain.AgentChat, []domain.AgentChatFolder, error) {
	current, err := u.loadChat(ctx, workspaceID, chatID)
	if err != nil {
		return domain.AgentChat{}, nil, err
	}
	snapshot, err := u.snapshot(ctx, workspaceID)
	if err != nil {
		return domain.AgentChat{}, nil, err
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	if mErr := u.checkMove(ctx, snapshot, workspaceID, chatID, destination); mErr != nil {
		return domain.AgentChat{}, nil, mErr
	}
	// Read BEFORE the plan is mutated: this is the only moment the lineage the
	// chat has been living under is still recoverable, and the comparison against
	// what it lands on is what decides whether anything happened worth recording.
	inherited := snapshot.chatLineage(chatID)
	u.replace(snapshot, chatID, current.ParentID, destination, in.Order)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.AgentChat{}, nil, err
	}
	u.noteNewAncestors(ctx, chatID, inherited, snapshot.chatLineage(chatID))
	return *snapshot.placedChat(chatID), written, nil
}

// noteNewAncestors writes the move into the chat's own conversation when, and
// only when, it actually gained a chat to read.
//
// GAINED, not merely changed. A chat dragged between two folders under the same
// parent reads exactly what it read a moment ago — folders are transparent — and
// announcing a context change there would be announcing one that did not happen.
// A chat dragged OUT from under a parent gains nothing and is likewise silent:
// what it no longer reads is already answered by its next spawn resolving an
// empty lineage, and there is no new context to date the start of.
//
// A failed note is logged and never returned. The rows have already moved by the
// time this runs, so failing the drag here would report an error for a move that
// happened and stands; and the relationship itself rides on ParentID, so what is
// lost is the line in the record, not the behaviour it describes.
func (u *chatFolderUsecase) noteNewAncestors(
	ctx context.Context,
	chatID string,
	inherited []string,
	lineage []string,
) {
	if !gained(inherited, lineage) {
		return
	}
	if err := u.agent.NoteThreadLineage(ctx, chatID, lineage); err != nil {
		slog.WarnContext(ctx, "agent chat folder: record new thread lineage",
			"err", err, "chat_id", chatID)
	}
}

// gained reports whether lineage names a chat that inherited did not — the test
// for "this chat now reads something it was not reading before".
func gained(
	inherited []string,
	lineage []string,
) bool {
	had := make(map[string]bool, len(inherited))
	for _, id := range inherited {
		had[id] = true
	}
	return slices.ContainsFunc(lineage, func(id string) bool { return !had[id] })
}

func (u *chatFolderUsecase) DeleteChat(
	ctx context.Context,
	chatID string,
) (ChatDeletion, error) {
	current, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return ChatDeletion{}, fmt.Errorf("agent chat folder: delete chat %s: %w", chatID, err)
	}
	snapshot, err := u.snapshot(ctx, current.WorkspaceID)
	if err != nil {
		return ChatDeletion{}, err
	}
	chats, folders := snapshot.subtree(chatID)
	chats = append(chats, chatID)
	if err := u.purgeAll(ctx, snapshot, chats); err != nil {
		return ChatDeletion{}, err
	}
	if err := u.removeAll(ctx, snapshot, folders); err != nil {
		return ChatDeletion{}, err
	}
	snapshot.plan.Reorder(current.ParentID, "", -1)
	shifted, err := u.persist(ctx, snapshot)
	if err != nil {
		return ChatDeletion{}, err
	}
	return ChatDeletion{Chats: chats, Folders: folders, Shifted: shifted}, nil
}

// purgeAll erases each chat in order and takes it out of the plan as it goes, so
// the densify that follows counts only the rows that survived.
func (u *chatFolderUsecase) purgeAll(
	ctx context.Context,
	snapshot *treeSnapshot,
	ids []string,
) error {
	for _, id := range ids {
		if err := u.agent.PurgeChat(ctx, id); err != nil {
			return fmt.Errorf("agent chat folder: purge chat %s: %w", id, err)
		}
		snapshot.dropChat(id)
	}
	return nil
}

// removeAll deletes each folder caught inside a purged subtree. They are removed
// rather than promoted because the level that would have held them is gone: a
// folder whose only reason to exist was ordering one chat's threads has nothing
// left to order.
func (u *chatFolderUsecase) removeAll(
	ctx context.Context,
	snapshot *treeSnapshot,
	ids []string,
) error {
	for _, id := range ids {
		if err := u.folders.Delete(ctx, id); err != nil {
			return fmt.Errorf("agent chat folder: delete %s: %w", id, err)
		}
		snapshot.drop(id)
	}
	return nil
}

// replace moves one row to its destination and leaves BOTH affected levels
// dense: the one it joined, and — only when it actually changed level — the one
// it left. Leaving every level dense after every move is what makes the next
// drop index mean what it says.
func (u *chatFolderUsecase) replace(
	snapshot *treeSnapshot,
	id string,
	origin string,
	destination string,
	requested *int,
) {
	target := placementTarget(requested, snapshot, origin, destination, id)
	snapshot.plan.SetParent(id, destination)
	snapshot.plan.Reorder(destination, id, target)
	if destination != origin {
		snapshot.plan.Reorder(origin, "", -1)
	}
}

// checkMove refuses a move onto a container that does not exist in the
// workspace, belongs to another workspace, or lies inside the moved row's own
// subtree.
func (u *chatFolderUsecase) checkMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	workspaceID string,
	id string,
	destination string,
) error {
	if destination == id {
		return fmt.Errorf("agent chat folder: move %s onto itself: %w", id, ErrCycle)
	}
	if err := u.checkContainer(ctx, snapshot, workspaceID, destination); err != nil {
		return err
	}
	if snapshot.plan.Reaches(destination, id) {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, ErrCycle)
	}
	return nil
}

// checkContainer validates a parent id: "" is the panel root, and anything else
// must be one of this workspace's folders or chats. A row that exists under a
// DIFFERENT workspace is reported as a cross-workspace edge rather than as a
// missing row, because the two are fixed in different ways.
func (u *chatFolderUsecase) checkContainer(
	ctx context.Context,
	snapshot *treeSnapshot,
	workspaceID string,
	parentID string,
) error {
	if parentID == "" {
		return nil
	}
	if snapshot.folder(parentID) != nil || snapshot.chat(parentID) != nil {
		return nil
	}
	elsewhere, err := u.folders.FindByKey(ctx, parentID)
	if err != nil {
		return fmt.Errorf("agent chat folder: resolve parent %s: %w", parentID, err)
	}
	if elsewhere != nil {
		return fmt.Errorf("agent chat folder: parent %s is in another workspace: %w", parentID, ErrCrossWorkspace)
	}
	return u.checkChatContainer(ctx, workspaceID, parentID)
}

// checkChatContainer resolves a parent id the workspace's own rows did not
// answer for against the CHAT aggregate, which is keyed globally. A chat in
// another workspace is a cross-workspace edge; a lookup that fails is surfaced
// as it came, so a read failure reaches the caller as one rather than as a
// confident "no such row" the user would go looking for.
//
// A chat that resolves to THIS workspace is accepted rather than refused, and
// that case is real: the keyed read heals the chat read model for the one id it
// was asked about, while the workspace list only heals a model that is entirely
// empty — so the authoritative answer here can name a row the snapshot's list
// did not carry. Refusing it would reject a drop onto a chat the user can see.
func (u *chatFolderUsecase) checkChatContainer(
	ctx context.Context,
	workspaceID string,
	parentID string,
) error {
	chat, err := u.chats.GetChat(ctx, parentID)
	if err != nil {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, err)
	}
	if chat.WorkspaceID == workspaceID {
		return nil
	}
	return fmt.Errorf(
		"agent chat folder: parent %s belongs to workspace %s, not %s: %w",
		parentID, chat.WorkspaceID, workspaceID, ErrCrossWorkspace,
	)
}

// load resolves a folder and refuses one belonging to another workspace, which
// is a NOT-FOUND rather than a cross-workspace refusal: the caller addressed a
// row that does not exist in the scope it asked in, and any other answer would
// confirm the existence of a row it may not touch. The check lives here rather
// than in the handler so every caller shares one rule.
func (u *chatFolderUsecase) load(
	ctx context.Context,
	workspaceID string,
	id string,
) (domain.AgentChatFolder, error) {
	row, err := u.folders.FindByKey(ctx, id)
	if err != nil {
		return domain.AgentChatFolder{}, fmt.Errorf("agent chat folder: get %s: %w", id, err)
	}
	if row == nil || row.WorkspaceID != workspaceID {
		return domain.AgentChatFolder{}, fmt.Errorf("agent chat folder: %s: %w", id, apperr.ErrNotFound)
	}
	return *row, nil
}

// loadChat resolves a chat and refuses one anchored to another workspace. It is
// a NOT-FOUND rather than a cross-workspace refusal: the caller addressed a row
// that does not exist in the scope it asked in, and answering otherwise would
// tell it that a chat it may not touch exists.
func (u *chatFolderUsecase) loadChat(
	ctx context.Context,
	workspaceID string,
	chatID string,
) (domain.AgentChat, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return domain.AgentChat{}, fmt.Errorf("agent chat folder: get chat %s: %w", chatID, err)
	}
	if chat.WorkspaceID != workspaceID {
		return domain.AgentChat{}, fmt.Errorf(
			"agent chat folder: chat %s is not in workspace %s: %w", chatID, workspaceID, apperr.ErrNotFound,
		)
	}
	return chat, nil
}

// snapshot reads one workspace's folders and chats as of a single moment. Both
// reads are workspace-scoped: the folder query is pushed down to SQL, and the
// chat read model serves the workspace slice natively.
func (u *chatFolderUsecase) snapshot(
	ctx context.Context,
	workspaceID string,
) (*treeSnapshot, error) {
	folders, err := u.folders.FindWhere(ctx, domain.AgentChatFolder{WorkspaceID: workspaceID})
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: snapshot: folders: %w", err)
	}
	chats, err := u.chats.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: snapshot: chats: %w", err)
	}
	return newTreeSnapshot(folders, chats), nil
}

// persist writes exactly the rows the plan touched and returns the FOLDER rows
// among them, which the caller has to broadcast itself. The chat rows need no
// such handling: their write is an aggregate command, so the hub projection
// broadcasts each one on the way through.
func (u *chatFolderUsecase) persist(
	ctx context.Context,
	snapshot *treeSnapshot,
) ([]domain.AgentChatFolder, error) {
	ids := snapshot.plan.Dirty()
	written := make([]domain.AgentChatFolder, 0, len(ids))
	for _, id := range ids {
		row, err := u.writeRow(ctx, snapshot, id)
		if err != nil {
			return nil, err
		}
		if row != nil {
			written = append(written, *row)
		}
	}
	return written, nil
}

func (u *chatFolderUsecase) writeRow(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
) (*domain.AgentChatFolder, error) {
	if row := snapshot.placedFolder(id); row != nil {
		if err := u.folders.Save(ctx, *row); err != nil {
			return nil, fmt.Errorf("agent chat folder: save %s: %w", id, err)
		}
		return row, nil
	}
	row := snapshot.placedChat(id)
	if row == nil {
		return nil, nil
	}
	if _, err := u.chats.SetPlacement(ctx, id, row.ParentID, row.Order); err != nil {
		return nil, fmt.Errorf("agent chat folder: place chat %s: %w", id, err)
	}
	return nil, nil
}

// without drops the subject row from a written set, leaving the collateral the
// caller broadcasts alongside it.
func without(
	rows []domain.AgentChatFolder,
	id string,
) []domain.AgentChatFolder {
	out := make([]domain.AgentChatFolder, 0, len(rows))
	for _, row := range rows {
		if row.ID != id {
			out = append(out, row)
		}
	}
	return out
}

// placementTarget resolves the index a moved row should land at: the caller's
// explicit request, its current index when it is only being re-parented in
// place, or the end of the destination when it arrives from elsewhere.
func placementTarget(
	requested *int,
	snapshot *treeSnapshot,
	origin string,
	destination string,
	id string,
) int {
	if requested != nil {
		return *requested
	}
	if origin == destination {
		return snapshot.plan.IndexOf(destination, id)
	}
	return snapshot.plan.NextSlot(destination)
}

func cleanName(
	name string,
) (string, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return "", ErrNameRequired
	}
	return clean, nil
}
