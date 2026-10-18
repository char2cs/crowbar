// Package agentchat is the asynx event-sourced repository for the chat aggregate
// (domain.Chat): a thread, its title and turn state, and nothing about the process
// talking to it. Mutations retry on optimistic concurrency; reads use the projection.
package chat

import "errors"

// ErrNotFound is returned when no chat exists for a requested id or provider
// session. The read-model store keeps its own local sentinel (to avoid an
// import cycle back into this package) and event_store.go bridges it to this
// one via mapNotFound, so every EventStore caller sees this single sentinel.
var ErrNotFound = errors.New("agentchat: not found")
