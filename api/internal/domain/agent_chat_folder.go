package domain

// AgentChatFolder is the Chats panel's ORGANISATION node: a named box a user
// drops chats and other folders into. It is workspace-scoped and, like the
// sidebar's Folder, it is the only row in that tree not backed by anything a
// process owns — no runner, no PTY, no ledger, no turns.
//
// That is why it is a plain GORM row rather than an event-sourced aggregate,
// exactly as Folder is: it has no concurrent OCC-sensitive write path, no
// reactor, and no process teardown to sequence, so the aggregate machinery would
// be pure cost. AutoMigrate creates the table; there is no migration code.
//
// It holds no turns, and THAT is what lets it live anywhere a chat can —
// including inside a chat, where it orders that chat's threads. A folder can
// never be mistaken for a thread and can never change what one reads, so the
// indent keeps one meaning at every level. Context lineage is the ancestor chain
// filtered to chats: the walk steps straight through a folder.
//
// ParentID names whatever the folder hangs off: a chat id, another folder's id,
// or "" for the panel root.
//
// Order is a dense index within ParentID's sibling space, which folders SHARE
// with chats: the two row kinds interleave at every level, so the panel merges
// both sets and sorts them on this one field.
type AgentChatFolder struct {
	ID          string `gorm:"primaryKey" json:"id"`
	WorkspaceID string `gorm:"index" json:"workspaceId"`
	ParentID    string `json:"parentId,omitempty"`
	Name        string `json:"name"`
	Order       int    `json:"order"`
}

// TableName is the GORM table this row persists to.
func (AgentChatFolder) TableName() string {
	return "agent_chat_folders"
}
