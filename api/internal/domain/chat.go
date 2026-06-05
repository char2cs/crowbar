package domain

import "time"

// Chat is the conversation aggregate; only its lifecycle is event-sourced here —
// turn content is deferred to the Agentic Bridge spike (00 §5.4, 01 §2).
type Chat struct {
	ID        string     `json:"id"`
	WsID      string     `json:"wsId"`
	Title     string     `json:"title"`
	ParentID  string     `json:"parentId,omitempty"`
	Status    ChatStatus `json:"status"`
	Type      ChatType   `json:"type"`
	CreatedAt time.Time  `json:"createdAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}
