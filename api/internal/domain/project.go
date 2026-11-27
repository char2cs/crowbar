package domain

import "time"

type Project struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	LastActivity  time.Time `json:"lastActivity"`
	Order         int       `json:"order"`
	AvatarHasIcon bool      `json:"avatarHasIcon"`
	AvatarVersion int64     `json:"avatarVersion,omitempty"`
	AvatarEmoji   string    `json:"avatarEmoji,omitempty"`
	Deleting      bool      `json:"deleting,omitempty"`
}

func (Project) TableName() string {
	return "projects"
}
