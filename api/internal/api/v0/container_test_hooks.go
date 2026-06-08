//go:build integration

package v0

import (
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

// WaitWorkspacesRegistered blocks until a workspaces WS client registers.
func (c *Container) WaitWorkspacesRegistered() { c.workspaces.WaitRegistered() }

// WaitNWorkspacesRegistered blocks until exactly n workspaces WS clients register.
func (c *Container) WaitNWorkspacesRegistered(
	n int,
) {
	c.workspaces.WaitNRegistered(n)
}

// WaitChatsRegistered blocks until a chats WS client registers.
func (c *Container) WaitChatsRegistered() { c.chats.WaitRegistered() }

// WaitNChatsRegistered blocks until exactly n chats WS clients register.
func (c *Container) WaitNChatsRegistered(
	n int,
) {
	c.chats.WaitNRegistered(n)
}

// WaitFilesRegistered blocks until a files WS client registers.
func (c *Container) WaitFilesRegistered() { c.files.WaitRegistered() }

// WaitNFilesRegistered blocks until exactly n files WS clients register.
func (c *Container) WaitNFilesRegistered(
	n int,
) {
	c.files.WaitNRegistered(n)
}

// WaitGitRegistered blocks until a git WS client registers.
func (c *Container) WaitGitRegistered() { c.git.WaitRegistered() }

// WaitNGitRegistered blocks until exactly n git WS clients register.
func (c *Container) WaitNGitRegistered(
	n int,
) {
	c.git.WaitNRegistered(n)
}

// WaitLSPRegistered blocks until an LSP WS client registers.
func (c *Container) WaitLSPRegistered() { c.lsp.WaitRegistered() }

// WaitNLSPRegistered blocks until exactly n LSP WS clients register.
func (c *Container) WaitNLSPRegistered(
	n int,
) {
	c.lsp.WaitNRegistered(n)
}

// PushLSP injects a diagnostics event directly into the LSP broadcaster,
// bypassing the engine OnDiagnostics callback.
func (c *Container) PushLSP(
	evt lspdomain.DiagnosticsEvent,
) {
	c.lsp.Push(evt)
}

