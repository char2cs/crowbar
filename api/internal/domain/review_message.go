package domain

import "time"

// ReviewMessage is one append-only message inside a ReviewThread (09 §3).
//
// ProviderID and ChatID attribute an agent-written message to the vendor CLI
// that wrote it and to the conversation it came out of, so the review UI can
// show WHICH agent left a finding and offer a way back to the chat that produced
// it. Both are opaque ids resolved by the client against the providers and chats
// it already holds; nothing here or downstream may branch on their values.
//
// Both are omitempty and both are empty for a human message — a person has
// neither a provider nor an originating chat. They are also empty on every
// message written before they existed: a ReviewThread is event-sourced but each
// event embeds the whole aggregate as a JSON blob, so historical messages
// unmarshal with the zero value forever and there is no backfill. Anything that
// reads them must degrade to the pre-attribution rendering when they are absent,
// because that is the common case and always will be.
type ReviewMessage struct {
	ID         string    `json:"id"`
	Author     string    `json:"author,omitempty"`
	IsAgent    bool      `json:"isAgent"`
	ProviderID string    `json:"providerId,omitempty"`
	ChatID     string    `json:"chatId,omitempty"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"createdAt"`
}
