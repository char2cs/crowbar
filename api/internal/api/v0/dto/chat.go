package dto

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatDTO is the wire shape of a Chat: the conversation aggregate that backs a
// chat-sidebar row (00 §5.4). It carries the lean set of fields the sidebar
// renders — id, owning workspace, status spinner, title, fork parent, and
// creation time — without leaking persistence-only fields.
type ChatDTO struct {
	ID        string            `json:"id"`
	WsID      string            `json:"wsId"`
	Status    domain.ChatStatus `json:"status"`
	Title     string            `json:"title,omitempty"`
	ParentID  string            `json:"parentId,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
}

// ChatDTOFrom converts a domain Chat into its wire DTO.
func ChatDTOFrom(
	chat domain.Chat,
) ChatDTO {
	return ChatDTO{
		ID:        chat.ID,
		WsID:      chat.WsID,
		Status:    chat.Status,
		Title:     chat.Title,
		ParentID:  chat.ParentID,
		CreatedAt: chat.CreatedAt,
	}
}

// ChatDTOList converts a slice of domain Chats into wire DTOs, returning a
// non-nil empty slice when the input is empty so the envelope carries [].
func ChatDTOList(
	chats []domain.Chat,
) []ChatDTO {
	dtos := make([]ChatDTO, 0, len(chats))
	for _, chat := range chats {
		dtos = append(dtos, ChatDTOFrom(chat))
	}
	return dtos
}
