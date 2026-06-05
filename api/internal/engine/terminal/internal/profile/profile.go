// Package profile resolves PTY launch parameters from a TerminalProfile.
package profile

import (
	"os"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Resolved holds the computed PTY launch parameters.
type Resolved struct {
	Shell   string
	CWD     string
	Startup []string
}

// Resolve derives shell, working directory, and startup commands from p.
// Shell fallback: $SHELL env var → /bin/sh.
// CWD fallback: workspaceDir.
func Resolve(
	p *domain.TerminalProfile,
	workspaceDir string,
) Resolved {
	shell := resolveShell(p)
	cwd := resolveCWD(p, workspaceDir)
	return Resolved{
		Shell:   shell,
		CWD:     cwd,
		Startup: resolveStartup(p),
	}
}

func resolveShell(
	p *domain.TerminalProfile,
) string {
	if p != nil && p.Shell != "" {
		return p.Shell
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

func resolveCWD(
	p *domain.TerminalProfile,
	workspaceDir string,
) string {
	if p != nil && p.StartupDirectory != "" {
		return p.StartupDirectory
	}
	return workspaceDir
}

func resolveStartup(
	p *domain.TerminalProfile,
) []string {
	if p == nil {
		return nil
	}
	return p.StartupCommands
}
