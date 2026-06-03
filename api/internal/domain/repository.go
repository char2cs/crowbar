package domain

import "time"

type Repository struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	ProjectID string    `gorm:"index" json:"project_id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`      // absolute path on disk
	CreatedAt time.Time `json:"created_at"`
}
