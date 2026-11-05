package domain

import "time"

// Project is the org-level node grouping repositories (00 §5.1).
type Project struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	LastActivity time.Time `json:"lastActivity"`
	// Order is the project's dense index within the sidebar. AutoMigrate adds the
	// column; rows written before it existed default to 0 and fall back to the id
	// tiebreak, which the first reorder replaces with a dense sequence.
	Order int `json:"order"`
	// The project's icon, on a Repository avatar's three-state model (emoji > on-disk
	// image > default). The default is the sidebar's Library glyph, so unset is both empty.
	AvatarHasIcon bool `json:"avatarHasIcon"`
	// AvatarVersion increments every time the on-disk icon bytes change, so the
	// DTO's otherwise-stable proxy URL gets a query param that moves and the
	// webview's image cache stops serving the icon that was replaced.
	AvatarVersion int64  `json:"avatarVersion,omitempty"`
	AvatarEmoji   string `json:"avatarEmoji,omitempty"`
	// Deleting is the durable intent of a delete, written before any of the
	// teardown runs; it is one-way, and boot re-drives every row carrying it.
	Deleting bool `json:"deleting,omitempty"`
}

func (Project) TableName() string {
	return "projects"
}
