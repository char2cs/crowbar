package ws

import "github.com/gin-gonic/gin"

// StreamDef declares how a Broadcaster routes, serializes, filters, and
// snapshots a stream of T (03 §1, §1a).
//
// ScopeKey, OnSubscribe, and OnUnsubscribe are the optional lazy-lifecycle
// hooks (03 §6). When ScopeKey is set, Handle derives a scope string from the
// request and calls OnSubscribe after a successful client registration and
// OnUnsubscribe after the client is removed. They drive refcounted per-scope
// resources such as the FileWatcher and LSP servers. When the hooks are nil the
// Broadcaster behaves exactly as without them (no regression).
type StreamDef[T any] struct {
	Namespace     func(T) string
	Serialize     func(T) ([]byte, error)
	Filters       []FilterDef[T]
	Snapshot      func() []T
	ScopeKey      func(*gin.Context) string
	OnSubscribe   func(scope string)
	OnUnsubscribe func(scope string)
}

// FilterDef is an optional query-param predicate over a stream value.
type FilterDef[T any] struct {
	Param   string
	Extract func(T) string
	Match   func(param, value string) bool
	Default string
}
