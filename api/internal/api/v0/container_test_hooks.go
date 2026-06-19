//go:build integration

package v0

import (
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

// WaitProjectsRegistered blocks until a projects WS client registers.
func (c *Container) WaitProjectsRegistered() { c.projects.WaitRegistered() }

// WaitNProjectsRegistered blocks until exactly n projects WS clients register.
func (c *Container) WaitNProjectsRegistered(
	n int,
) {
	c.projects.WaitNRegistered(n)
}

// WaitReposRegistered blocks until a repos WS client registers.
func (c *Container) WaitReposRegistered() { c.repos.WaitRegistered() }

// WaitNReposRegistered blocks until exactly n repos WS clients register.
func (c *Container) WaitNReposRegistered(
	n int,
) {
	c.repos.WaitNRegistered(n)
}

// WaitWorkspacesRegistered blocks until a workspaces WS client registers.
func (c *Container) WaitWorkspacesRegistered() { c.workspaces.WaitRegistered() }

// WaitThreadsRegistered blocks until a threads WS client registers.
func (c *Container) WaitThreadsRegistered() { c.threads.WaitRegistered() }

// WaitNThreadsRegistered blocks until exactly n threads WS clients register.
func (c *Container) WaitNThreadsRegistered(
	n int,
) {
	c.threads.WaitNRegistered(n)
}

// WaitTerminalsRegistered blocks until a terminals WS client registers.
func (c *Container) WaitTerminalsRegistered() { c.terminals.WaitRegistered() }

// WaitNTerminalsRegistered blocks until exactly n terminals WS clients register.
func (c *Container) WaitNTerminalsRegistered(
	n int,
) {
	c.terminals.WaitNRegistered(n)
}

// WaitNWorkspacesRegistered blocks until exactly n workspaces WS clients register.
func (c *Container) WaitNWorkspacesRegistered(
	n int,
) {
	c.workspaces.WaitNRegistered(n)
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
