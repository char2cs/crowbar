package hub

import (
	"sync"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Hub fans out domain broadcasts to all registered Subscribers. It implements
// WebSocketHub so the app layer can broadcast through it.
type Hub struct {
	mu          sync.RWMutex
	subscribers []Subscriber
}

// NewHub constructs an empty Hub.
func NewHub() *Hub {
	return &Hub{}
}

// Register adds a Subscriber to the fan-out set.
func (h *Hub) Register(
	s Subscriber,
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribers = append(h.subscribers, s)
}

// BroadcastProject fans a ProjectDTO out to every subscriber (spec §5).
func (h *Hub) BroadcastProject(
	p dto.ProjectDTO,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushProject(p)
	}
}

// BroadcastRepo fans a RepoDTO out to every subscriber (spec §5).
func (h *Hub) BroadcastRepo(
	r dto.RepoDTO,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushRepo(r)
	}
}

// BroadcastWorkspace fans a WorkspaceDTO out to every subscriber (spec §5). The
// merge-eligibility overlay is resolved by the producer before this call.
func (h *Hub) BroadcastWorkspace(
	w dto.WorkspaceDTO,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushWorkspace(w)
	}
}

// BroadcastThread fans a ThreadDTO out to every subscriber (spec §5).
func (h *Hub) BroadcastThread(
	t dto.ThreadDTO,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushThread(t)
	}
}

// BroadcastTerminalSession fans a TerminalSessionDTO out to every subscriber
// (spec §5).
func (h *Hub) BroadcastTerminalSession(
	s dto.TerminalSessionDTO,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, sub := range h.subscribers {
		sub.PushTerminalSession(s)
	}
}

// BroadcastGit fans a GitStatus out to every subscriber (Class B, 03 §2).
func (h *Hub) BroadcastGit(
	wsID string,
	status gitdomain.GitStatus,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushGit(wsID, status)
	}
}

// BroadcastFile fans a FileChangeEvent out to every subscriber (Class B, 03 §2).
func (h *Hub) BroadcastFile(
	evt domain.FileChangeEvent,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushFile(evt)
	}
}

// BroadcastAgentChatEvent fans one chat snapshot frame out on the chat feed —
// every chat and runner lifecycle change, carrying the chat's full versioned
// snapshot (see usecases/chat/internal/snapshot).
func (h *Hub) BroadcastAgentChatEvent(ev dto.AgentChatEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChatEvent(ev)
	}
}

// BroadcastAgentChatPromptSettled fans out the edge where a delivered prompt is
// retired without ever having produced a turn, on the same workspace-scoped feed
// as BroadcastAgentChat.
//
// Fed by the terminal-wait detector, like the wait edge and for the same reason:
// the fact is derived from a live PTY's screen joined against the chat's busy
// state and its delivery journal, so no aggregate's event log can emit it.
//
// consumed says whether anything proved the provider took the prompt, and rides
// the frame because a client cannot derive it: both cases look identical in the
// ledger (neither produced a turn). A client holding the user's typed text may
// discard it only when consumed is true.
func (h *Hub) BroadcastAgentChatPromptSettled(
	chatID string,
	workspaceID string,
	requestID string,
	consumed bool,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChatPromptSettled(chatID, workspaceID, requestID, consumed)
	}
}

// BroadcastAgentChatMessageDelta fans a growing assistant message out on the same
// workspace-scoped feed as every other fact about a conversation.
//
// Unlike the other chat broadcasts this one is HIGH FREQUENCY — roughly 1.4 per
// second per streaming chat — and it is deliberately the only thing in this
// feature that never touches durable storage. A partial message is a view, not a
// record; the ledger gets the message once, when it is finished.
// kind says WHICH stream this text belongs to: the empty string (or "answer")
// for what the agent is saying, "reasoning" for what it is thinking on the way
// there. Both are transient views of the same shape; only the answer is ever
// recorded, and a client renders the two differently.
func (h *Hub) BroadcastAgentChatMessageDelta(
	chatID string,
	workspaceID string,
	messageID string,
	text string,
	kind string,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChatMessageDelta(chatID, workspaceID, messageID, text, kind)
	}
}

// BroadcastAgentChatPlan fans the agent's own running to-do list out on the same
// workspace-scoped feed as every other fact about a conversation.
//
// Restated WHOLESALE on every update, like the streamed message's text-so-far:
// the newest list is the entire truth, so a client that missed a frame is correct
// again on the next one. Like that one it is deliberately never stored — a plan
// for a turn in progress is a view, not a record.
func (h *Hub) BroadcastAgentChatPlan(
	chatID string,
	workspaceID string,
	steps []agents.PlanStep,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChatPlan(chatID, workspaceID, steps)
	}
}

// BroadcastAgentChatTelemetry fans the provider's newest usage report for a
// chat out on the same workspace-scoped feed, so the gauge moves when the
// provider reports instead of on a client poll.
func (h *Hub) BroadcastAgentChatTelemetry(
	chatID string,
	workspaceID string,
	report agents.Telemetry,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChatTelemetry(chatID, workspaceID, report)
	}
}

// BroadcastAgentChatCompaction fans the live compact_pre/compact_post edge out
// on the same workspace-scoped feed as every other fact about a conversation.
//
// Fed directly by the hook ingress rather than by an aggregate projection, for
// the same reason BroadcastAgentChatTerminalWait is: the ledger's own record of
// this fact is born already resolved (a /compact never opens a tracked turn,
// so commands.Interrupt's idle-chat handling resolves it in the same event
// that creates it — see turn.Turns.compactionStatus's doc comment), so no
// aggregate event log can emit a live "started" edge for it. active is the
// whole answer, both ways round, same as TerminalWait's presence/absence.
func (h *Hub) BroadcastAgentChatCompaction(
	chatID string,
	workspaceID string,
	active bool,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChatCompaction(chatID, workspaceID, active)
	}
}

// BroadcastAgentChatFolder fans a CHAT FOLDER lifecycle event
// (folder_created/folder_updated/folder_deleted) out on the SAME workspace-scoped
// agent-chat WebSocket as BroadcastAgentChat. A chat folder is a plain GORM row
// with no aggregate projection to ride, so the mutating handler calls this
// itself right after the write — exactly as the sidebar's folder handlers call
// BroadcastFolder.
//
// A second socket for folders would buy nothing and would have to be kept in
// ORDER with the first: one gesture moves both kinds (they share a sibling
// space), so a folder frame and the chat frames its densify caused have to
// arrive in a sequence the client can reconcile.
//
// The frame names the folder and nothing more. The Chats feed carries no
// snapshot, so a client cannot hold folders from it alone; putting the row on
// the frame would create a second way to learn a placement, and the two would
// disagree the first time a frame was dropped.
func (h *Hub) BroadcastAgentChatFolder(
	folderID string,
	workspaceID string,
	kind string,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChatFolder(folderID, workspaceID, kind)
	}
}

var _ WebSocketHub = (*Hub)(nil)
