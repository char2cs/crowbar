//go:build integration

package v0

import (
	"github.com/gin-gonic/gin"

	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

// ProjectsHandle exposes the Projects broadcaster's WS upgrade handler so tests
// can mount it on an ad-hoc route before the hierarchical router lands (W7).
func (c *Container) ProjectsHandle(
	ctx *gin.Context,
) {
	c.projects.Handle(ctx)
}

// ReposHandle exposes the Repos broadcaster's WS upgrade handler so tests can
// mount it on an ad-hoc route before the hierarchical router lands (W7).
func (c *Container) ReposHandle(
	ctx *gin.Context,
) {
	c.repos.Handle(ctx)
}

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

// AgentChatsHandle exposes the agent-chat broadcaster's WS upgrade handler so
// tests can mount it on an ad-hoc route, without the chat group's own
// resolveChatWorktree guard.
func (c *Container) AgentChatsHandle(
	ctx *gin.Context,
) {
	c.agentChats.Handle(ctx)
}

// WaitAgentChatsRegistered blocks until an agent-chat WS client registers.
func (c *Container) WaitAgentChatsRegistered() { c.agentChats.WaitRegistered() }

// WaitNAgentChatsRegistered blocks until exactly n agent-chat WS clients register.
func (c *Container) WaitNAgentChatsRegistered(
	n int,
) {
	c.agentChats.WaitNRegistered(n)
}

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
