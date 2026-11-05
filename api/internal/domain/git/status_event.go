package git

// GitStatusEvent carries a workspace's working-tree status on the Git topic. WsID
// scopes the broadcast to one workspace; only the embedded Status is serialized, so
// the wire shape stays a bare GitStatus.
type GitStatusEvent struct {
	WsID   string    `json:"wsId"`
	Status GitStatus `json:"status"`
}
