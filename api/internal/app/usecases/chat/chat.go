package chat

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/conversation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/defaultlevel"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/manifestfetch"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/provider"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// ChatUsecase owns the chat aggregate — identity, title, model selection, hard
// deletion — and every read off its conversation record. It starts no processes:
// a chat is readable whether or not a CLI ever ran on it.
type ChatUsecase interface {
	// ChatSnapshot is a chat's versioned snapshot — see internal/snapshot.
	ChatSnapshot(ctx context.Context, chatID string) (ChatSnapshot, error)

	// ChatSnapshotsOf is ChatSnapshot for many chats, in order, read together.
	ChatSnapshotsOf(ctx context.Context, chatIDs []string) ([]ChatSnapshot, error)

	// MintChat creates an empty, dormant chat in a workspace and returns its id.
	// providerID is recorded durably ("" for a placeholder that never runs one);
	// surface is the view it is born on ("" for the provider's default).
	MintChat(
		ctx context.Context,
		chatID string,
		workspaceID string,
		providerID string,
		surface string,
	) (string, error)

	// RenameChat retitles a chat by source: "derived" never overwrites an existing
	// title, "agent" never overwrites a locked one, anything else wins and locks.
	RenameChat(
		ctx context.Context,
		chatID, title, source string,
	) error

	// RenameByRunner retitles the chat a live runner is on; a runner that is
	// not live is problem.ErrNotFound.
	RenameByRunner(
		ctx context.Context,
		runnerID, title, source string,
	) error

	// PurgeChat hard-deletes a chat: the aggregate, its conversation record, its
	// telemetry, its conversation history and its on-disk footprint, retiring
	// every CLI still on it.
	PurgeChat(
		ctx context.Context,
		chatID string,
	) error

	// OpenFeed is the chat's row feed from since: see Usecase.OpenFeed.
	OpenFeed(ctx context.Context, chatID string, since int64) (<-chan FeedFrame, error)
	// ReadBlob is a payload the chat's rows name: see Usecase.ReadBlob.
	ReadBlob(ctx context.Context, chatID, ref string) ([]byte, error)

	// ListChats returns every chat the daemon knows, across all workspaces.
	ListChats(
		ctx context.Context,
	) ([]domain.Chat, error)

	// BackfillChatTypes records Type on every row minted before it existed. It
	// runs once per install at boot and counts the rows it could not replay.
	BackfillChatTypes(
		ctx context.Context,
	) (skipped int, err error)

	// UntypedChats lists the replayable chats BackfillChatTypes left untyped.
	UntypedChats(
		ctx context.Context,
	) ([]string, error)

	// ListChatsByWorkspace returns one workspace's chats.
	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)

	// ListChatsInRepo returns every conversation-typed row whose cwd walk lands
	// on a workspace in repoID — see repo_scope.go.
	ListChatsInRepo(
		ctx context.Context,
		repoID string,
	) ([]domain.Chat, error)

	// CwdWorkspaceID answers where a row's CLI runs: the workspace of the
	// nearest ancestor-or-self carrying one — see repo_scope.go.
	CwdWorkspaceID(
		ctx context.Context,
		chatID string,
	) (string, bool, error)

	// GetChat reads one chat aggregate.
	GetChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)

	// SetChatSelection records the model and effort the chat's next CLI is to be
	// launched with, refusing a value the resolved provider does not declare with
	// problem.ErrInvalidArgument.
	SetChatSelection(
		ctx context.Context,
		chatID string,
		model string,
		effort string,
	) error

	// ReadChatLog renders a chat's whole conversation as speaker/body turns. It is
	// the read behind the get_chat_log tool one agent uses to read another chat.
	ReadChatLog(
		ctx context.Context,
		chatID string,
	) ([]agenttools.ChatTurn, error)

	// NoteThreadLineage records, in the chat's own conversation, that it has been
	// moved under new parents from this point on. It appends nothing to a chat
	// that has said nothing yet.
	NoteThreadLineage(
		ctx context.Context,
		chatID string,
		ancestors []string,
	) error

	// Ancestors returns the CHAT ancestors of chatID, nearest parent first — what a
	// thread inherits. Folders are transparent to it; empty at the panel root.
	Ancestors(
		ctx context.Context,
		chatID string,
	) ([]string, error)

	// AssembleHandoff renders the first ask and recent turns (HandoffWindow) into the
	// document an incoming CLI is spawned with; "" when there is nothing to hand over.
	AssembleHandoff(
		ctx context.Context,
		chatID string,
	) (string, error)
}

var _ ChatUsecase = (*Usecase)(nil)

// Usecase is the chat feature: one type whose surface is split into narrow ports by
// file (chat record, turn, runner, answers, providers) because the responsibilities call
// each other in a cycle. The work lives in internal/ components that never import a peer.
type Usecase struct {
	// The stores and seams the answer path reads directly. Everything else this
	// type needs, it needs only long enough to build a component out of it.
	chats  agentchat.EventStore
	agents engineagents.Agents
	// ledger is every chat's record, and blobs the tool payloads its rows name.
	ledger *ledgerstore.Store
	blobs  *ledgerstore.Blobs
	ws     WorkspaceReader
	// home resolves the crowbar home each chat's own directory hangs off.
	home     func() (string, error)
	worktree WorktreeCreator
	// folders/nodes let the placement-tree walks (fork parent, cwd, repo scope) see
	// past a folder, which is not a Chat row. May be nil: walks then see Chat rows only.
	folders   TreeFolders
	nodes     TreeNodes
	repoRoots TreeRepoRoots
	// answers is the desk of relays currently BLOCKED on a human. It is in memory
	// because a slot describes a live hook process holding a live provider gate
	// open; see answers.go.
	answers *answerdesk.Desk
	// asks closes the chat's asks, for the desk and for a person's answer.
	asks askResolver
	// tools is kept after construction because New alone fills its self-ports, and a
	// nil ToolAccess FAILS OPEN (a provider's disabled Tools switch silently back on);
	// keeping it lets that wiring be guarded.
	tools agenttools.Deps

	// snapshots is the one owner of every chat's versioned snapshot.
	snapshots *snapshot.Snapshots
	// feed is the hub, bound at sweep start; telemetry publishes through it.
	feed atomic.Pointer[ChatFeed]
	// feeds are the per-chat row feeds.
	feeds *feeds

	// The components, each owning one responsibility; the delegating methods are
	// the whole of what reaches them.
	conversations *conversation.Conversations
	// recorder decides and records the rows of every hook the ingress places.
	recorder *ingress.Recorder
	runners  *runner.Runners
	// ingress is the one door every hook comes in through, over the recorder.
	ingress       *ingress.Ingress
	providers     *provider.Providers
	defaultLevel  *defaultlevel.DefaultLevel
	manifestFetch *manifestfetch.ManifestFetch
}

// The chat record. A chat exists, and is readable, whether or not a CLI has ever
// run on it — everything here is answerable with no runner in sight.

// MintChat creates an empty chat in a workspace, recording the provider it is
// born on, and returns its id.
func (u *Usecase) MintChat(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	surface string,
) (string, error) {
	return u.conversations.MintChat(ctx, chatID, workspaceID, providerID, surface)
}

// RenameChat retitles a chat under the user > agent > derived precedence.
func (u *Usecase) RenameChat(
	ctx context.Context,
	chatID, title, source string,
) error {
	return u.conversations.RenameChat(ctx, chatID, title, source)
}

// RenameByRunner retitles the chat a live runner is on.
func (u *Usecase) RenameByRunner(
	ctx context.Context,
	runnerID, title, source string,
) error {
	return u.conversations.RenameByRunner(ctx, runnerID, title, source)
}

// PurgeChat hard-deletes a chat and retires every CLI still on it. It translates the
// repository's not-found into problem.ErrNotFound, which tree.Agent's contract uses
// for "nothing to purge".
func (u *Usecase) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	err := u.conversations.PurgeChat(ctx, chatID)
	if errors.Is(err, agentchat.ErrNotFound) {
		return fmt.Errorf("%w: %w", err, problem.ErrNotFound)
	}
	if err == nil {
		u.feeds.deleted(chatID)
	}
	return err
}

// ListChats returns every chat in the daemon.
func (u *Usecase) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	return u.conversations.ListChats(ctx)
}

// BackfillChatTypes implements ChatUsecase.
func (u *Usecase) BackfillChatTypes(
	ctx context.Context,
) (int, error) {
	return u.conversations.BackfillChatTypes(ctx)
}

// UntypedChats implements ChatUsecase.
func (u *Usecase) UntypedChats(
	ctx context.Context,
) ([]string, error) {
	return u.conversations.UntypedChats(ctx)
}

// ListChatsByWorkspace returns the chats anchored to one workspace.
func (u *Usecase) ListChatsByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.Chat, error) {
	return u.conversations.ListChatsByWorkspace(ctx, workspaceID)
}

// GetChat returns one chat by id.
func (u *Usecase) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	return u.conversations.GetChat(ctx, id)
}

// SetChatSelection pins the model and effort the chat's next CLI launches with and
// records whichever CHANGED. It runs under the runners' spawn hold so it cannot race a
// concurrent SubmitPrompt staging its own selection.
func (u *Usecase) SetChatSelection(
	ctx context.Context,
	chatID, model, effort string,
) error {
	return u.runners.SetChatSelection(ctx, chatID, model, effort)
}

// ReadChatLog returns the chat's turns as the tool surface renders them.
func (u *Usecase) ReadChatLog(
	ctx context.Context,
	chatID string,
) ([]agenttools.ChatTurn, error) {
	return u.conversations.ReadChatLog(ctx, chatID)
}

// NoteThreadLineage records, in the chat's own log, which chats it was threaded
// under.
func (u *Usecase) NoteThreadLineage(
	ctx context.Context,
	chatID string,
	ancestors []string,
) error {
	return u.conversations.NoteThreadLineage(ctx, chatID, ancestors)
}

// AssembleHandoff renders the chat's ledger into the prior context a freshly
// spawned CLI can be given.
func (u *Usecase) AssembleHandoff(
	ctx context.Context,
	chatID string,
) (string, error) {
	return u.conversations.AssembleHandoff(ctx, chatID)
}

// Ancestors returns the CHAT ancestors of chatID, nearest parent first — what a
// thread inherits.
func (u *Usecase) Ancestors(
	ctx context.Context,
	chatID string,
) ([]string, error) {
	return u.conversations.Ancestors(ctx, chatID)
}

// ChatSnapshot returns chatID's current versioned snapshot — the same answer,
// from the same owner, every chat frame carries.
func (u *Usecase) ChatSnapshot(ctx context.Context, chatID string) (ChatSnapshot, error) {
	return u.snapshots.Get(ctx, chatID)
}

// ChatSnapshotsOf is ChatSnapshot for many chats at once, in order: one hold of
// the owner's lock and one placement read for the whole list.
func (u *Usecase) ChatSnapshotsOf(ctx context.Context, chatIDs []string) ([]ChatSnapshot, error) {
	return u.snapshots.GetMany(ctx, chatIDs)
}

// Snapshots is the owner itself, for a test that wires the fanout over it.
func (u *Usecase) Snapshots() *ChatSnapshots { return u.snapshots }
