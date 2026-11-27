package safego

import (
	"log/slog"
	"runtime/debug"
)

func Go(name string, fn func()) {
	go func() {
		defer Recover(name)
		fn()
	}()
}

func Recover(name string) {
	if r := recover(); r != nil {
		slog.Error("recovered panic in goroutine",
			"goroutine", name,
			"panic", r,
			"stack", string(debug.Stack()),
		)
	}
}

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
