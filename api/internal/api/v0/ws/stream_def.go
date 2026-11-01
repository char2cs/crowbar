package ws

// StreamDef declares how a Broadcaster routes, serializes, filters, and
// snapshots a stream of T (03 §1, §1a).
type StreamDef[T any] struct {
	Namespace func(T) string
	Serialize func(T) ([]byte, error)
	Filters   []FilterDef[T]
	Snapshot  func(scope string) []T
	// FlatNamespace marks a stream whose Namespace is a bare leaf id rather than
	// a hierarchical "p/r/w" path: it is scoped only by its explicit Filters,
	// since a "p/r" prefix can never match a bare id.
	FlatNamespace bool
}

// FilterDef is an optional query-param predicate over a stream value.
//
// Required refuses a client that resolves no value for Param — path, query and
// Default all empty — instead of dropping the filter for that client. Without
// it an unparameterised subscriber is silently over-subscribed to EVERY event
// on the stream rather than unsubscribed from all of them, which is the
// difference between a chat-scoped client seeing one workspace and seeing all
// of them.
type FilterDef[T any] struct {
	Param    string
	Extract  func(T) string
	Match    func(param, value string) bool
	Default  string
	Required bool
}
