package domain

import "time"

type Client struct {
	ID        string     `gorm:"primaryKey" json:"id"`
	TokenHash string     `gorm:"uniqueIndex" json:"-"`
	Name      string     `json:"name"`
	PairedAt  time.Time  `json:"pairedAt"`
	LastSeen  time.Time  `json:"lastSeen"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}

func (Client) TableName() string {
	return "clients"
}
