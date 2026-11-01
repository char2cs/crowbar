package dto

// AgentChatEvent is the frame on the agent-chat lifecycle WebSocket: what
// changed about a chat, a runner or a folder, and the workspace it belongs to.
// It carries entity facts only; a chat's messages ride its own feed.
type AgentChatEvent struct {
	ChatID      string `json:"chatId"`
	WorkspaceID string `json:"workspaceId"`
	// RepoID and ProjectID are the row's derived ground, which scope the
	// repo-scoped mount; empty reaches every subscriber of the wider scope
	// (see matchScopeOrUnscoped).
	RepoID    string `json:"repoId,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
	Kind      string `json:"kind"`
	// RunnerID is set only on the runner kinds (started/exited).
	RunnerID string `json:"runnerId,omitempty"`
	// FolderID is set only on the folder kinds: a client re-reads the
	// workspace's folders on one, as it does on a reconnect.
	FolderID string `json:"folderId,omitempty"`
	// Worktree rides the worktree_state kind: the git state of the worktree
	// ChatID owns, carried whole so no client refetches it.
	Worktree *ChatWorktreeDTO `json:"worktree,omitempty"`
	// Chat is the chat's snapshot as of this frame, and Version its version:
	// a client applies it only when Version is newer than the one it holds.
	Chat    *AgentChatDTO `json:"chat,omitempty"`
	Version int64         `json:"version,omitempty"`
	// Telemetry is the provider's newest usage report, on the telemetry kind.
	Telemetry *AgentTelemetryDTO `json:"telemetry,omitempty"`
}

// AgentChatKindWorktreeState announces that the git state of the worktree a
// chat owns has moved; a chat that owns no worktree never produces one.
const AgentChatKindWorktreeState = "worktree_state"

// AgentChatKindTelemetry announces the provider's newest usage report for the
// chat, carried whole on the frame.
const AgentChatKindTelemetry = "telemetry"
