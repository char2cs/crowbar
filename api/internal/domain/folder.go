package domain

// Folder is a sidebar folder's identity: its name and which repo's tree it
// organises. RepoID == "" means a project-home folder. Its POSITION lives on
// Node, not here — a rename is a single, low-stakes CRUD fact with no
// drag-time densify race to guard and no motivated undo case, so unlike Node
// this is plain GORM, not event-sourced (2026-09-08
// sidebar-placement-unification design §2.2).
type Folder struct {
	ID     string `gorm:"primaryKey"`
	Name   string
	RepoID string
}

func (Folder) TableName() string {
	return "folders"
}
