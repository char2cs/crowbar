package lsp

import (
	"strings"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

func (e *engine) evictSnapshot(
	wsID string,
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.snap, wsID)
	prefix := wsID + "|"
	for key := range e.versions {
		if strings.HasPrefix(key, prefix) {
			delete(e.versions, key)
		}
	}
}

func (e *engine) OnDiagnostics(
	fn func(domlsp.DiagnosticsEvent),
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.userCB = fn
}

func (e *engine) onDiagnostics(
	event domlsp.DiagnosticsEvent,
) {
	e.mu.Lock()
	e.snap[event.WsID] = event.Diagnostics
	cb := e.userCB
	e.mu.Unlock()
	if cb != nil {
		cb(event)
	}
}

func (e *engine) DiagnosticsSnapshot(
	wsID string,
) []domlsp.Diagnostic {
	e.mu.Lock()
	defer e.mu.Unlock()
	diags, ok := e.snap[wsID]
	if !ok {
		return []domlsp.Diagnostic{}
	}
	return diags
}
