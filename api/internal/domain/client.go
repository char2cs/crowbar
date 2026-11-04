package domain

import "time"

// Client is a device paired to talk to this daemon over the network. The
// credential is a bearer token; only its SHA-256 hash is ever persisted —
// the raw token is shown to the pairing client exactly once, at redeem time,
// and never stored anywhere.
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
