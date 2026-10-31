//go:build integration

package v0

import (
	"github.com/gin-gonic/gin"
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
