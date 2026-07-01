package git

// GitStatusEvent carries a workspace's working-tree status on the Git topic.
//
// The WsID scopes the broadcast to a single workspace (03 §3) so a subscriber
// at .../workspaces/A/git/status never receives workspace B's status. The Git
// broadcaster filters on WsID but serializes only the embedded Status, so the
// wire shape on the Git WebSocket remains a bare GitStatus — matching the REST
// snapshot of the dual-served .../workspaces/:wsId/git/status route (02 §2.6).
type GitStatusEvent struct {
	WsID   string    `json:"wsId"`
	Status GitStatus `json:"status"`
}
