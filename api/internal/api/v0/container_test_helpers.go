//go:build integration

package v0

import (
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

// ContainerBridge is a plain struct of closures that gives test packages
// access to the private broadcasters inside Container without adding exported
// methods to the production type.  Obtain one via Container.TestBridge().
type ContainerBridge struct {
	WaitWorkspacesRegistered  func()
	WaitNWorkspacesRegistered func(n int)
	WaitChatsRegistered       func()
	WaitNChatsRegistered      func(n int)
	WaitFilesRegistered       func()
	WaitNFilesRegistered      func(n int)
	WaitGitRegistered         func()
	WaitNGitRegistered        func(n int)
	WaitLSPRegistered         func()
	WaitNLSPRegistered        func(n int)
	PushLSP                   func(evt lspdomain.DiagnosticsEvent)
	PushFile                  func(evt domain.FileChangeEvent)
	PushGit                   func(wsID string, status gitdomain.GitStatus)
}

// TestBridge returns a ContainerBridge wired to the private broadcasters.
// This is the single integration-only surface on Container; it is intentionally
// named to make its test-only purpose obvious.
func (c *Container) TestBridge() ContainerBridge {
	return ContainerBridge{
		WaitWorkspacesRegistered:  c.workspaces.WaitRegistered,
		WaitNWorkspacesRegistered: c.workspaces.WaitNRegistered,
		WaitChatsRegistered:       c.chats.WaitRegistered,
		WaitNChatsRegistered:      c.chats.WaitNRegistered,
		WaitFilesRegistered:       c.files.WaitRegistered,
		WaitNFilesRegistered:      c.files.WaitNRegistered,
		WaitGitRegistered:         c.git.WaitRegistered,
		WaitNGitRegistered:        c.git.WaitNRegistered,
		WaitLSPRegistered:         c.lsp.WaitRegistered,
		WaitNLSPRegistered:        c.lsp.WaitNRegistered,
		PushLSP:  c.lsp.Push,
		PushFile: c.files.Push,
		PushGit: func(wsID string, status gitdomain.GitStatus) {
			c.git.Push(gitdomain.GitStatusEvent{WsID: wsID, Status: status})
		},
	}
}
