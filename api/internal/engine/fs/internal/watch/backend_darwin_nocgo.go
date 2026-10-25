//go:build darwin && !cgo

package watch

// The darwin watcher is an FSEvents stream and needs cgo; there is no kqueue
// fallback, which would hold a descriptor per watched file. Build with
// CGO_ENABLED=1.
var _ = darwinWatcherRequiresCGOEnabled1
