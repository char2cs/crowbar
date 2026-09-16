package git

// GitStatusEvent carries a workspace's working-tree status on the Git topic.
//
// The WsID scopes the broadcast to a single workspace (03 §3) so a subscriber
// at .../workspaces/A/git/status never receives workspace B's status. The Git
// broadcaster filters on WsID but serializes only the embedded Status, so the
// wire shape on the Git WebSocket remains a bare GitStatus — matching the REST
// snapshot of the dual-served .../workspaces/:wsId/git/status route (02 §2.6).
//
// ChatIDs is the same scoping question answered for the chat-scoped route
// /v0/chats/:chatId/git/status: every chat whose worktree resolves to WsID, as
// of this push (spec §7.4). Git is the shared bucket — one worktree, one
// answer — so a status a write through ONE chat produced is news for every
// sibling chat holding that worktree, and carrying the set on the event is what
// lets one Push reach all of them in a single fan-out pass. It is routing, not
// payload: the Serialize above never writes it, and a consumer is never handed
// a workspace's chat roster.
type GitStatusEvent struct {
	WsID    string    `json:"wsId"`
	ChatIDs []string  `json:"-"`
	Status  GitStatus `json:"status"`
}
