package domain

import "time"

// Branch is a git branch entry (04 §3).
type Branch struct {
	Name           string     `json:"name"`
	IsCurrent      bool       `json:"isCurrent"`
	IsRemote       bool       `json:"isRemote"`
	Ahead          *int       `json:"ahead,omitempty"`
	Behind         *int       `json:"behind,omitempty"`
	LastCommitDate *time.Time `json:"lastCommitDate,omitempty"`
}
