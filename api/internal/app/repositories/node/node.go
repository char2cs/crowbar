// Package node is the asynx event-sourced repository for domain.Node — the ONE
// aggregate that owns every sidebar row's position (ParentID/Order), at every
// tree level (chat, folder, workspace, repo). It mirrors agentchat's structure
// (api/internal/app/repositories/chat) almost exactly, narrowed to a
// position-only aggregate: the EventStore (event_store.go) is the sole
// repository — mutations dispatch the command layer with optimistic-concurrency
// retry, reads delegate to the store-package read-model projection.
package node

import "errors"

// ErrNotFound is returned when no node exists for a requested id. The read-model
// store keeps its own local sentinel (to avoid an import cycle back into this
// package) and event_store.go bridges it to this one via mapNotFound, so every
// EventStore caller sees this single sentinel.
var ErrNotFound = errors.New("node: not found")
