package ws

import "github.com/gin-gonic/gin"

// StreamDef declares how a Broadcaster routes, serializes, filters, and
// snapshots a stream of T (03 §1, §1a).
//
// ScopeKey, OnSubscribe, and OnUnsubscribe are the optional lazy-lifecycle
// hooks (03 §6) driving refcounted per-scope resources such as the FileWatcher
// and LSP servers. When ScopeKey is set, each client holds one scope from
// registration to removal: its bound filter's value if it has one (moved on
// Rebind: the new scope is acquired, then the old released), else ScopeKey's
// answer for the request. A scope of "" is never acquired.
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
	// CoalesceKey opts a stream into latest-wins delivery: when set and it
	// reports ok for a value, a client's per-key pending slot is OVERWRITTEN
	// rather than queued, so a slow client is never starved and never
	// disconnected for this value's kind — a later write for the same key
	// simply supersedes the earlier, still-undelivered one.
	//
	// This is correct ONLY for a stream whose own values are already
	// "the full state so far", so a receiver who misses several superseded
	// writes is exactly as correct as one who saw every one of them — e.g. an
	// assistant message broadcast as "everything said so far" rather than as
	// an increment (see hub.BroadcastAgentChatMessageDelta's own doc
	// comment: "a client that missed a frame is correct again on the next
	// one"). It must NOT be set for a stream whose values are individually
	// meaningful deltas or lifecycle edges (an increment, a state
	// transition) — coalescing those loses information a later value can
	// never reconstruct. Nil (the default) keeps every stream's existing,
	// disconnect-on-overflow behavior exactly as it was — see
	// TestPush_SlowConsumer_DisconnectsInsteadOfDroppingForever, which this
	// field does not change for any stream that leaves it unset.
	//
	// Values sharing NO key (ok == false) go through the ordinary bounded
	// queue untouched, and are always delivered strictly before any
	// currently-pending coalesced value (see client.coalesce's own doc
	// comment) — coalescing trades ordering only among values that already
	// declared they don't need it, never against the rest of the stream.
	CoalesceKey func(T) (key string, ok bool)
}

// FilterDef is an optional query-param predicate over a stream value.
//
// Required refuses a client that resolves no value for Param — path, query and
// Default all empty — instead of dropping the filter for that client. Without
// it an unparameterised subscriber is silently over-subscribed to EVERY event
// on the stream rather than unsubscribed from all of them, which is the
// difference between a chat-scoped client seeing one workspace and seeing all
// of them.
//
// Resolve makes the filter BOUND: the client's Param value (a chat id) is
// resolved to the value Extract is matched against (its workspace id) at
// connect and again on every Broadcaster.Rebind, which passes all clients'
// values in one call. A value resolving to "" matches nothing; one left out
// of the answer keeps the client's current binding.
type FilterDef[T any] struct {
	Param    string
	Extract  func(T) string
	Match    func(param, value string) bool
	Default  string
	Required bool
	Resolve  func(params []string) map[string]string
}
