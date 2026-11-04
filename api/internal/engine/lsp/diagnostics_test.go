package lsp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

func TestDiagnosticsSnapshot_EmptyUntilEvent(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)

	assert.Empty(t, e.DiagnosticsSnapshot(ws))

	// Spawn the server so its diagnostics callback is wired through the manager.
	_, err := e.Completion(context.Background(), ws, tree, goF, pos())
	require.NoError(t, err)

	cb := fake.diagCallback()
	require.NotNil(t, cb)

	cb(domlsp.DiagnosticsEvent{WsID: ws, Diagnostics: []domlsp.Diagnostic{{Message: "boom"}}})

	snap := e.DiagnosticsSnapshot(ws)
	require.Len(t, snap, 1)
	assert.Equal(t, "boom", snap[0].Message)
	assert.Empty(t, e.DiagnosticsSnapshot("other"))
}

func TestOnDiagnostics_UserCallbackAlsoInvoked(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)

	var got domlsp.DiagnosticsEvent
	e.OnDiagnostics(func(ev domlsp.DiagnosticsEvent) { got = ev })

	_, err := e.Completion(context.Background(), ws, tree, goF, pos())
	require.NoError(t, err)

	cb := fake.diagCallback()
	require.NotNil(t, cb)
	cb(domlsp.DiagnosticsEvent{WsID: ws, Diagnostics: []domlsp.Diagnostic{{Message: "x"}}})

	assert.Equal(t, ws, got.WsID)
	require.Len(t, got.Diagnostics, 1)
}
