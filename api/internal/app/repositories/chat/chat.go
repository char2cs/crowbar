// Package chat is the asynx event-sourced repository for the chat aggregate
// (domain.Chat): a thread, its title and turn state, and nothing about the process
// talking to it. Mutations retry on optimistic concurrency; reads use the projection.
package chat

import (
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// ErrNotFound is returned when no chat exists for a requested id or provider
// session. The read-model store's own sentinel (kept to avoid an import cycle)
// is mapped to this one, so every EventStore caller sees a single sentinel.
var ErrNotFound = problem.New(problem.NotFound, "agentchat: not found")
