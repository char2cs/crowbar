package agent

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/chatlog"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
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
	) (chatlog.Page, error)

	// NoteThreadLineage records, in the chat's own conversation, that it has been
	// moved under new parents from this point on. It appends nothing to a chat
	// that has said nothing yet.
	NoteThreadLineage(
		ctx context.Context,
		chatID string,
		ancestors []string,
	) error

	// AssembleHandoff renders the whole conversation into the document an
	// incoming CLI is spawned with. It returns the empty string when there is
	// nothing to hand over.
	AssembleHandoff(
		ctx context.Context,
		chatID string,
	) (string, error)
}

var _ ChatUsecase = (*chatUsecase)(nil)

type chatUsecase struct {
	chats   agentchat.EventStore
	runners agentrunner.EventStore
	// activity is the conversation record: turns, tool calls, subagents and
	// interruptions. Every read this type serves comes off it.
	activity  agentactivity.EventStore
	telemetry *telemetryStore
	agents    engineagents.Agents
	ws        WorkspaceReader
	// lineage answers "what does this chat read" at spawn time. See ChatLineage
	// and threadContext.
	lineage ChatLineage
	// home is the app-config crowbar-home resolver, NOT a wsId lookup: it resolves
	// the descriptor catalog a chat's model/effort selection is validated against.
	home func() (string, error)
	work *chatWorkStates
	// spawns is the per-chat gate the USER-INITIATED spawn paths share. PurgeChat
	// takes it for the same reason they do — a delete racing a switch would
	// otherwise start a CLI onto a chat that has just been forgotten. See chatGate.
	spawns *chatGate
	// runner is the runner lifecycle, reached for the one thing a hard delete owes
	// the processes on the chat: retiring them.
	runner *runnerUsecase
}

func newChatUsecase(
	chats agentchat.EventStore,
	runners agentrunner.EventStore,
	activity agentactivity.EventStore,
	telemetry *telemetryStore,
	agents engineagents.Agents,
	ws WorkspaceReader,
	lineage ChatLineage,
	home func() (string, error),
	work *chatWorkStates,
	spawns *chatGate,
) *chatUsecase {
	return &chatUsecase{
		chats:     chats,
		runners:   runners,
		activity:  activity,
		telemetry: telemetry,
		agents:    agents,
		ws:        ws,
		lineage:   lineage,
		home:      home,
		work:      work,
		spawns:    spawns,
	}
}
