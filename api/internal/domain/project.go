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
}

func (Project) TableName() string {
	return "projects"
}
