package chat

import (
	"context"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/conversation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/defaultlevel"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/provider"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatUsecase owns the chat aggregate — its identity, title, model selection
// and hard deletion — and every read served off the conversation record it
// accumulates.
//
// It starts no processes. A chat exists, and is readable, whether or not a CLI
// has ever run on it: minting one, renaming it and reading its log are all
// answerable with no runner in sight, which is why they are separable from the
// runner lifecycle at all.
type ChatUsecase interface {
	// MintChat creates an empty chat in a workspace and returns its id. No CLI is
	// started: the chat is dormant until a runner is placed on it.
	MintChat(
		ctx context.Context,
		workspaceID string,
	) (string, error)

	// RenameChat retitles a chat, honouring where the title came from: a
	// "derived" title never overwrites an existing one, an "agent" title never
	// overwrites a locked one, and anything else is a manual rename that wins and
	// locks.
	RenameChat(
		ctx context.Context,
		chatID, title, source string,
	) error

	// RenameByRunner retitles whichever chat a runner is currently placed on. A
	// runner placed nowhere renames nothing.
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

	// ListChats returns every chat the daemon knows, across all workspaces.
	ListChats(
		ctx context.Context,
	) ([]domain.Chat, error)

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
	// apperr.ErrInvalidArgument.
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

	// ReadMessages pages the conversation record, newest page last. after and
	// before are mutually exclusive cursors.
	ReadMessages(
		ctx context.Context,
		chatID string,
		after, before, limit int,
	) (domain.LedgerPage, error)

	// NoteThreadLineage records, in the chat's own conversation, that it has been
	// moved under new parents from this point on. It appends nothing to a chat
	// that has said nothing yet.
	NoteThreadLineage(
		ctx context.Context,
		chatID string,
		ancestors []string,
	) error

	// Ancestors returns the CHAT ancestors of chatID, nearest parent first —
	// what a thread inherits. Folders are transparent to it, so a thread filed
	// two folders deep under a chat inherits exactly what it would sitting
	// directly under it. Empty for a chat at the panel root.
	Ancestors(
		ctx context.Context,
		chatID string,
	) ([]string, error)

	// AssembleHandoff renders the conversation — capped to the most recent
	// turns, see RecentHandoffWindow — into the document an incoming CLI is
	// spawned with. It returns the empty string when there is nothing to hand
	// over.
	AssembleHandoff(
		ctx context.Context,
		chatID string,
	) (string, error)
	// Promote fills a bubble's empty workspace slot (model spec §4.2, promote.go).
	Promote(
		ctx context.Context,
		chatID string,
	) (domain.Chat, error)
}

var _ ChatUsecase = (*Usecase)(nil)

// Usecase is the chat feature: one type, whose methods are exposed across five
// files by responsibility — the chat record here, the turn in turn.go, the vendor
// CLI in runner.go, the human-in-the-loop in answers.go and the provider table in
// providers.go.
//
// Each of those five files opens with the port it satisfies and the sentinels its
// methods return, so a consumer reads one file to learn one responsibility. They
// are five ports and not one because a consumer should not be able to reach past
// what it needs: a route that lists chats has no business starting a CLI. The
// surface is split; the state is not.
//
// It is ONE type and not five because those five responsibilities are one
// surface that calls itself in a cycle: a purge retires the CLIs on the chat, a
// hook moves a runner, a switch waits on a turn only the hook path can end. Five
// types could only express that by binding each other after construction, which
// is what this replaced.
//
// The work itself is NOT here. It lives in four components under internal/, and
// those really are separate: each declares the narrow port it needs from a peer
// and never imports one (see layering_test.go). This type is where they are built,
// wired, and named for the outside — nothing more.
type Usecase struct {
	// The stores and seams the answer path reads directly. Everything else this
	// type needs, it needs only long enough to build a component out of it.
	chats       agentchat.EventStore
	runnerStore agentrunner.EventStore
	activity    agentactivity.EventStore
	agents      engineagents.Agents
	ws          WorkspaceReader
	worktree    WorktreeCreator
	// answers is the desk of relays currently BLOCKED on a human. It is in memory
	// because a slot describes a live hook process holding a live provider gate
	// open; see answers.go.
	answers *answerdesk.Desk
	// tools is the agent-facing capability surface. It is kept after construction
	// for one reason: its Chats, ChatLogs, Lineage and ToolAccess ports are this
	// type's own methods, so New is the only thing that can fill them, and a nil
	// ToolAccess FAILS OPEN — the per-provider Tools switch the user turned off
	// would be silently back on. Keeping the value is what lets that wiring be
	// guarded rather than assumed.
	tools agenttools.Deps

	// work is the SAME tracker sh.work hands conversations/turns/runners — see
	// Work in aliases.go.
	work *inflight.Work

	// The five components. Each owns one responsibility, and the delegating
	// methods in this file and the other five are the whole of what reaches them.
	conversations *conversation.Conversations
	turns         *turn.Turns
	runners       *runner.Runners
	providers     *provider.Providers
	defaultLevel  *defaultlevel.DefaultLevel
}

// The chat record. A chat exists, and is readable, whether or not a CLI has ever
// run on it — everything here is answerable with no runner in sight.

// MintChat creates an empty chat in a workspace and returns its id.
func (u *Usecase) MintChat(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	return u.conversations.MintChat(ctx, workspaceID)
}

// RenameChat retitles a chat under the user > agent > derived precedence.
func (u *Usecase) RenameChat(
	ctx context.Context,
	chatID, title, source string,
) error {
	return u.conversations.RenameChat(ctx, chatID, title, source)
}

// RenameByRunner retitles whichever chat a runner is currently placed on.
func (u *Usecase) RenameByRunner(
	ctx context.Context,
	runnerID, title, source string,
) error {
	return u.conversations.RenameByRunner(ctx, runnerID, title, source)
}

// PurgeChat hard-deletes a chat and retires every CLI still on it.
func (u *Usecase) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	return u.conversations.PurgeChat(ctx, chatID)
}

// ListChats returns every chat in the daemon.
func (u *Usecase) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	return u.conversations.ListChats(ctx)
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

// SetChatSelection pins the model and reasoning effort the chat's next CLI is
// launched with. Recording which of model/effort actually CHANGED —
// Crowbar's own doing, never something a provider reports — is
// recordSelectionChange's job, in turn.go beside the rest of what u.turns
// backs.
func (u *Usecase) SetChatSelection(
	ctx context.Context,
	chatID, model, effort string,
) error {
	// Read the CURRENT sticky selection before applying the change: this is
	// what tells a real change from a request that just restates what the
	// chat already had, and it's the only place that comparison can be made —
	// SetSelection itself always overwrites both fields unconditionally.
	before, beforeErr := u.conversations.ChatSelection(ctx, chatID, false)

	if err := u.conversations.SetChatSelection(ctx, chatID, model, effort); err != nil {
		return err
	}

	u.recordSelectionChange(ctx, chatID, before, beforeErr, model, effort)
	return nil
}

// ReadChatLog returns the chat's turns as the tool surface renders them.
func (u *Usecase) ReadChatLog(
	ctx context.Context,
	chatID string,
) ([]agenttools.ChatTurn, error) {
	return u.conversations.ReadChatLog(ctx, chatID)
}

// ReadMessages returns one page of the chat's turns.
func (u *Usecase) ReadMessages(
	ctx context.Context,
	chatID string,
	after, before, limit int,
) (domain.LedgerPage, error) {
	return u.conversations.ReadMessages(ctx, chatID, after, before, limit)
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
