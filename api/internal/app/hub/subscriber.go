package hub

import (
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Subscriber receives hub broadcasts. Implemented by the API WS handler set,
// which fans each entity DTO out to the matching per-entity broadcaster (spec
// §5). The entity topics carry their wire DTOs directly; the Git and File topics
// stay on their domain payloads (the broadcaster serialises them at the edge).
type Subscriber interface {
	PushProject(
		p dto.ProjectDTO,
	)
	PushRepo(
		r dto.RepoDTO,
	)
	PushWorkspace(
		w dto.WorkspaceDTO,
	)
	PushThread(
		t dto.ThreadDTO,
	)
	PushTerminalSession(
		s dto.TerminalSessionDTO,
	)
	PushGit(
		wsID string,
		status gitdomain.GitStatus,
	)
	PushFile(
		evt domain.FileChangeEvent,
	)
	// PushAgentChatPromptSettled receives the frame that says one delivered prompt
	// is over without having produced a turn, so a client holding it as pending can
	// let it go. It names the client's own request id, and whether anything proved
	// the provider actually took the prompt — a client may discard the user's text
	// only on consumed, never on the bare timeout that leaves it the last copy.
	PushAgentChatPromptSettled(
		chatID string,
		workspaceID string,
		requestID string,
		consumed bool,
	)
	// PushAgentChatMessageDelta receives one streamed text block as far as it has
	// been said, so a client can render it growing. Carries the text so far rather
	// than the increment, so a dropped frame costs nothing.
	//
	// kind names the stream: empty (or "answer") is the agent talking, "reasoning"
	// is the agent thinking. A reasoning stream is never recorded in the ledger —
	// it is a live view only, and it exists because a reasoning model spends most
	// of a hard turn emitting nothing else.
	PushAgentChatMessageDelta(
		chatID string,
		workspaceID string,
		messageID string,
		text string,
		kind string,
	)
	// PushAgentChatPlan receives the agent's own running to-do list for the turn,
	// restated wholesale. Never stored: a plan for a turn in progress is a view of
	// it, not a record of it.
	PushAgentChatPlan(
		chatID string,
		workspaceID string,
		steps []agents.PlanStep,
	)
	// PushAgentChatEvent receives one chat snapshot frame.
	PushAgentChatEvent(ev dto.AgentChatEvent)
	// PushAgentChatTelemetry receives the provider's newest usage report.
	PushAgentChatTelemetry(
		chatID string,
		workspaceID string,
		report agents.Telemetry,
	)
	// PushAgentChatCompaction receives the live compact_pre/compact_post edge —
	// a fact the ledger's own interruption record cannot carry live (see
	// hub.BroadcastAgentChatCompaction's own doc comment). active is the whole
	// answer both ways round: true on compact_pre, false on compact_post.
	PushAgentChatCompaction(
		chatID string,
		workspaceID string,
		active bool,
	)
	// PushAgentChatFolder receives a CHAT FOLDER lifecycle frame
	// (folder_created/folder_updated/folder_deleted). It carries the folder id and
	// nothing else: the Chats socket is a bare event feed with no snapshot, so a
	// frame here means "re-read this workspace's chat folders" — the same thing a
	// reconnect does, which is what makes the live and outage paths repair
	// identically.
	PushAgentChatFolder(
		folderID string,
		workspaceID string,
		kind string,
	)
}
