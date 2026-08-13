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

// BroadcastFolder fans a FolderDTO out to every subscriber (spec §5). Folders
// are a plain GORM row with no aggregate projection to ride, so the mutating
// handler calls this directly after the save — the same path the repo icon
// mutations already take.
func (h *Hub) BroadcastFolder(
	f dto.FolderDTO,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushFolder(f)
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
