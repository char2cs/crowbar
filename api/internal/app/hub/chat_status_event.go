package hub

import "github.com/char2cs/crowbar/api/internal/domain"

// ChatStatusEvent is the Chats-topic payload; chatId identifies the row (03 §4.2).
type ChatStatusEvent struct {
	ChatID string            `json:"chatId"`
	WsID   string            `json:"wsId"`
	Status domain.ChatStatus `json:"status"`
}
