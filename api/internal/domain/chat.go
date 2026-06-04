package domain

import "time"

// Chat is the conversation aggregate; only its status lifecycle is event-sourced
// here — turn content is deferred to the Agentic Bridge spike (00 §5.4).
type Chat struct {
	ID        string     `json:"id"`
	WsID      string     `json:"wsId"`
	Status    ChatStatus `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
}
