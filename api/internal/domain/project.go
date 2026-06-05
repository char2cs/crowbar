package domain

import "time"

// Project is the org-level node grouping repositories (00 §5.1).
type Project struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	LastActivity time.Time `json:"lastActivity"`
}

func (Project) TableName() string {
	return "projects"
}
