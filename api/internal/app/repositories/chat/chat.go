// Package agentchat is the asynx event-sourced repository for the Crowbar-owned agentic
// chat aggregate (domain.Chat) — a thread and its title and turn state, and NOTHING
// about the process talking to it. The CLI is the agentrunner aggregate; it points at a
// chat, and the chat never points back. The EventStore (event_store.go) is the sole
// repository: mutations dispatch the command layer with optimistic-concurrency retry,
// reads delegate to the store-package read-model projection. This aggregate is distinct
// from the dormant event-sourced domain.Chat.
package chat

import "errors"

// ErrNotFound is returned when no chat exists for a requested id or provider
// session. The read-model store keeps its own local sentinel (to avoid an
// import cycle back into this package) and event_store.go bridges it to this
// one via mapNotFound, so every EventStore caller sees this single sentinel.
var ErrNotFound = errors.New("agentchat: not found")
