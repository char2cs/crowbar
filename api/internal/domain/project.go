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
	// The project's icon, on the same three-state model a Repository's avatar
	// uses (emoji > on-disk image > default). A project's default is the Library
	// glyph the sidebar draws, not a generated letter tile, so there is no label
	// or colour to carry — an unset icon is simply both of these empty/false.
	AvatarHasIcon bool `json:"avatarHasIcon"`
	// AvatarVersion increments every time the on-disk icon bytes change, so the
	// DTO's otherwise-stable proxy URL gets a query param that moves and the
	// webview's image cache stops serving the icon that was replaced.
	AvatarVersion int64  `json:"avatarVersion,omitempty"`
	AvatarEmoji   string `json:"avatarEmoji,omitempty"`
}

func (Project) TableName() string {
	return "projects"
}
