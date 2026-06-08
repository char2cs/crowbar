//go:build integration

// Package v0test provides test helpers that wrap *v0.Container without
// polluting the production type with exported test methods.
package v0test

import (
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

// Wrapper gives integration tests access to the wait/push helpers for every
// broadcaster topic managed by a *v0.Container.
type Wrapper struct {
	bridge v0.ContainerBridge
}

// Wrap creates a Wrapper around c.  c must not be nil.
func Wrap(
	c *v0.Container,
) *Wrapper {
	return &Wrapper{bridge: c.TestBridge()}
}

// WaitWorkspacesRegistered blocks until at least one workspaces WebSocket
// client has registered with the broadcaster.
func (w *Wrapper) WaitWorkspacesRegistered() {
	w.bridge.WaitWorkspacesRegistered()
}

// WaitNWorkspacesRegistered blocks until exactly n workspaces WebSocket
// clients have registered.
func (w *Wrapper) WaitNWorkspacesRegistered(
	n int,
) {
	w.bridge.WaitNWorkspacesRegistered(n)
}

// WaitChatsRegistered blocks until at least one chats WebSocket client has
// registered with the broadcaster.
func (w *Wrapper) WaitChatsRegistered() {
	w.bridge.WaitChatsRegistered()
}

// WaitNChatsRegistered blocks until exactly n chats WebSocket clients have
// registered.
func (w *Wrapper) WaitNChatsRegistered(
	n int,
) {
	w.bridge.WaitNChatsRegistered(n)
}

// WaitFilesRegistered blocks until at least one files WebSocket client has
// registered with the broadcaster.
func (w *Wrapper) WaitFilesRegistered() {
	w.bridge.WaitFilesRegistered()
}

// WaitNFilesRegistered blocks until exactly n files WebSocket clients have
// registered.
func (w *Wrapper) WaitNFilesRegistered(
	n int,
) {
	w.bridge.WaitNFilesRegistered(n)
}

// WaitGitRegistered blocks until at least one git WebSocket client has
// registered with the broadcaster.
func (w *Wrapper) WaitGitRegistered() {
	w.bridge.WaitGitRegistered()
}

// WaitNGitRegistered blocks until exactly n git WebSocket clients have
// registered.
func (w *Wrapper) WaitNGitRegistered(
	n int,
) {
	w.bridge.WaitNGitRegistered(n)
}

// WaitLSPRegistered blocks until at least one LSP diagnostics WebSocket
// client has registered with the broadcaster.
func (w *Wrapper) WaitLSPRegistered() {
	w.bridge.WaitLSPRegistered()
}

// WaitNLSPRegistered blocks until exactly n LSP diagnostics WebSocket clients
// have registered.
func (w *Wrapper) WaitNLSPRegistered(
	n int,
) {
	w.bridge.WaitNLSPRegistered(n)
}

// PushLSP injects a diagnostics event directly into the LSP broadcaster,
// bypassing the engine OnDiagnostics callback.  Use this in integration tests
// when no real language server is attached.
func (w *Wrapper) PushLSP(
	evt lspdomain.DiagnosticsEvent,
) {
	w.bridge.PushLSP(evt)
}

// PushFile injects a FileChangeEvent directly into the files broadcaster,
// bypassing the OS watcher.  Use this in integration tests when deterministic
// event injection is required without a running file watcher.
func (w *Wrapper) PushFile(
	evt domain.FileChangeEvent,
) {
	w.bridge.PushFile(evt)
}

// PushGit injects a GitStatus directly into the git broadcaster,
// bypassing the file watcher and git write usecases.  Use this in integration
// tests when deterministic git status injection is required.
func (w *Wrapper) PushGit(
	wsID string,
	status gitdomain.GitStatus,
) {
	w.bridge.PushGit(wsID, status)
}
