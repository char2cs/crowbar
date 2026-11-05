package domain

import "time"

// Client is a device paired to talk to this daemon over the network by bearer token.
// Only the token's SHA-256 is persisted; the raw token is shown once, at redeem time.
type Client struct {
	ID string `gorm:"primaryKey" json:"id"`
	// TokenHash is the SHA-256 hex digest of the bearer token this client
	// presents on every request. Unique: two clients never share a token.
	TokenHash string     `gorm:"uniqueIndex" json:"-"`
	Name      string     `json:"name"`
	PairedAt  time.Time  `json:"pairedAt"`
	LastSeen  time.Time  `json:"lastSeen"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}

func (Client) TableName() string {
	return "clients"
}
