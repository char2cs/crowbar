package domain

// Folder is the sidebar's ORGANISATION node: a named box a user drops
// workspaces and other folders into. It is repo-scoped and it is the only
// sidebar row that is not backed by anything on disk — no branch, no worktree,
// no git status, no WorkspaceStatus.
//
// That is why it is a plain GORM row rather than a fifth event-sourced
// aggregate: it has no concurrent OCC-sensitive write path, no reactor, and no
// physical teardown to sequence, so the aggregate machinery (its own event log,
// snapshot store, and shard router) would be pure cost. AutoMigrate creates the
// table; there is no migration code.
//
// ParentID names whatever the folder hangs off: a workspace id, another folder's
// id, or "" for the repo root. A folder may sit under a PROTECTED branch — that
// is where most of them will live, hanging off `develop`.
//
// Order is a dense index within ParentID's sibling space, which folders SHARE
// with workspaces: the two row kinds interleave at every level, so the sidebar
// merges both sets and sorts them on this one field.
type Folder struct {
	ID        string `gorm:"primaryKey" json:"id"`
	RepoID    string `json:"repoId"`
	ProjectID string `json:"projectId"`
	ParentID  string `json:"parentId,omitempty"`
	Name      string `json:"name"`
	Order     int    `json:"order"`
}

func (Folder) TableName() string {
	return "folders"
}
