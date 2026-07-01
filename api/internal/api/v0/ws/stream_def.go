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
	Namespace func(T) string
	Serialize func(T) ([]byte, error)
	Filters   []FilterDef[T]
	Snapshot  func(scope string) []T
	// FlatNamespace marks a stream whose Namespace is a bare leaf id (e.g. the
	// git/files/lsp wsId) rather than a hierarchical "p/r/w" path. Such streams
	// are scoped only by their explicit Filters; the hierarchical client-scope
	// prefix (derived from the projectId/repoId/wsId path params their routes
	// now nest under) is not applied, since a "p/r" prefix can never match a
	// bare wsId namespace.
	FlatNamespace bool
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
