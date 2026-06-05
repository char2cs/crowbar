package ws

// StreamDef declares how a Broadcaster routes, serializes, filters, and
// snapshots a stream of T (03 §1, §1a).
type StreamDef[T any] struct {
	Namespace func(T) string
	Serialize func(T) ([]byte, error)
	Filters   []FilterDef[T]
	Snapshot  func() []T
}

// FilterDef is an optional query-param predicate over a stream value.
type FilterDef[T any] struct {
	Param   string
	Extract func(T) string
	Match   func(param, value string) bool
	Default string
}
