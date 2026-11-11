// Package safego runs detached goroutines behind a panic-recovery boundary. A panic
// on a bare goroutine would crash the long-lived daemon and drop every workspace's
// streams and terminals, so each one recovers, logs a stack and contains the failure.
package safego

import (
	"log/slog"
	"runtime/debug"
)

// Go spawns fn in a new goroutine behind a recovery boundary. A panic in fn is
// logged with its stack and contained instead of crashing the process.
func Go(name string, fn func()) {
	go func() {
		defer Recover(name)
		fn()
	}()
}

// Recover is the deferred panic boundary for a goroutine you spawn yourself
// (e.g. `go s.loop()`); place `defer safego.Recover("watcher.loop")` as the
// first statement of the goroutine body. A recovered panic is logged and swallowed.
func Recover(name string) {
	if r := recover(); r != nil {
		slog.Error("recovered panic in goroutine",
			"goroutine", name,
			"panic", r,
			"stack", string(debug.Stack()),
		)
	}
}

// RecoverFn is like Recover but invokes onPanic with the recovered value after
// logging, so the caller can surface the failure (e.g. as a failed operation)
// instead of letting it vanish.
func RecoverFn(name string, onPanic func(recovered any)) {
	if r := recover(); r != nil {
		slog.Error("recovered panic in goroutine",
			"goroutine", name,
			"panic", r,
			"stack", string(debug.Stack()),
		)
		if onPanic != nil {
			onPanic(r)
		}
	}
}
