// Package safego runs detached goroutines behind a panic-recovery boundary.
//
// Crowbar's daemon is a long-lived sidecar. A panic on a bare goroutine — an
// async git mutation, the WebSocket broadcaster fan-out, a file watcher, an LSP
// reader, a terminal pump — is not attributable to any HTTP request, so the
// runtime would crash the entire process, dropping every connected workspace's
// streams, terminals, and watchers at once. Every spawned goroutine must instead
// recover, log with a stack trace, and contain the failure to that unit of work.
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
// logging, so the caller can surface the failure (e.g. as a workspace lastError)
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
