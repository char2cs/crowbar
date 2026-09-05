package hub

import (
	"sync"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
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

// BroadcastAgentChat fans an agent-chat lifecycle event (created/segment_opened/
// segment_ended/session_bound/turn_started/turn_stopped/title_set/deleted) out
// to every subscriber. Fed solely by the agentchat hub projection, which derives
// the kind from the emitting command's event name and workspaceID from the
// reduced aggregate. workspaceID rides on every frame so the agent-chat WS
// StreamDef scopes the fan-out to the matching :wsId subscription (Task 3): this
// method pushes to every subscriber, and the per-subscription Filter drops frames
// whose WorkspaceID does not match the subscribed workspace.
//
// working is the chat's folded busy state as of this event, carried so the client
// never has to re-derive it from the kind — see store.BroadcastFunc.
func (h *Hub) BroadcastAgentChat(
	chatID string,
	workspaceID string,
	kind string,
	working bool,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChat(chatID, workspaceID, kind, working)
	}
}

// BroadcastAgentChatTerminalWait fans the terminal-wait edge out on the same
// workspace-scoped agent-chat feed as BroadcastAgentChat.
//
// Fed by the terminal-wait detector rather than by an aggregate projection, and it
// has to be: the fact is DERIVED from a live PTY's screen joined against the chat's
// busy state and its outstanding prompts, so no single aggregate's event log can
// emit it. Called only when the verdict MOVES — a chat parked for an hour produces
// one frame, not one per sweep.
//
// wait is nil on the clearing edge.
func (h *Hub) BroadcastAgentChatTerminalWait(
	chatID string,
	workspaceID string,
	wait *dto.AgentTerminalWaitDTO,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChatTerminalWait(chatID, workspaceID, wait)
	}
}

// BroadcastAgentChatPromptSettled fans out the edge where a delivered prompt is
// retired without ever having produced a turn, on the same workspace-scoped feed
// as BroadcastAgentChat.
//
// Fed by the terminal-wait detector, like the wait edge and for the same reason:
// the fact is derived from a live PTY's screen joined against the chat's busy
// state and its delivery journal, so no aggregate's event log can emit it.
func (h *Hub) BroadcastAgentChatPromptSettled(
	chatID string,
	workspaceID string,
	requestID string,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChatPromptSettled(chatID, workspaceID, requestID)
	}
}

// BroadcastAgentChatMessageDelta fans a growing assistant message out on the same
// workspace-scoped feed as every other fact about a conversation.
//
// Unlike the other chat broadcasts this one is HIGH FREQUENCY — roughly 1.4 per
// second per streaming chat — and it is deliberately the only thing in this
// feature that never touches durable storage. A partial message is a view, not a
// record; the ledger gets the message once, when it is finished.
func (h *Hub) BroadcastAgentChatMessageDelta(
	chatID string,
	workspaceID string,
	messageID string,
	text string,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChatMessageDelta(chatID, workspaceID, messageID, text)
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

// BroadcastAgentRunner fans an agent-RUNNER lifecycle event
// (started/session_bound/moved/displaced/exited) out to every subscriber. Fed solely by
// the agentrunner hub projection, which derives the kind from the emitting command's
// event name.
//
// The frame carries PLACEMENT, never liveness: chatID is the chat the runner is
// pointed at AS OF this event, so a `moved` frame names the chat the CLI moved
// INTO — which is precisely what the frontend needs to re-point the tab that was
// following that runner. An `exited` frame means the live row is gone and that
// chat is now dormant.
//
// runnerID rides along so a client can tell WHICH CLI moved (a chat can be
// handed between runners); workspaceID scopes the fan-out exactly as it does for
// BroadcastAgentChat — this method pushes to every subscriber and the
// per-subscription Filter drops frames for other workspaces.
func (h *Hub) BroadcastAgentRunner(
	runnerID string,
	workspaceID string,
	chatID string,
	kind string,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentRunner(runnerID, workspaceID, chatID, kind)
	}
}

var _ WebSocketHub = (*Hub)(nil)
